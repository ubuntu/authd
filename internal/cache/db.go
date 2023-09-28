// Package cache handles transaction with an underlying database to cache user and group informations.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/ubuntu/decorate"
	"go.etcd.io/bbolt"
)

var (
	dbName          = "authd.db"
	dirtyFlagDbName = dbName + ".dirty"
)

const (
	userByNameBucketName   = "UserByName"
	userByIDBucketName     = "UserByID"
	groupByNameBucketName  = "GroupByName"
	groupByIDBucketName    = "GroupByID"
	userToGroupsBucketName = "UserToGroups"
	groupToUsersBucketName = "GroupToUsers"

	// defaultEntryExpiration is the amount of time the user is allowed on the cache without authenticating.
	// It's equivalent to 6 months.
	defaultEntryExpiration = time.Hour * 24 * 30 * 6

	// defaultCleanupInterval is the interval upon which the cache will be cleaned of expired users.
	defaultCleanupInterval = time.Hour * 24
)

var (
	allBuckets = [][]byte{
		[]byte(userByNameBucketName), []byte(userByIDBucketName),
		[]byte(groupByNameBucketName), []byte(groupByIDBucketName),
		[]byte(userToGroupsBucketName), []byte(groupToUsersBucketName)}
)

// Cache is our database API.
type Cache struct {
	db *bbolt.DB
	mu sync.RWMutex

	dirtyFlagPath  string
	procDir        string
	doClear        chan struct{}
	quit           chan struct{}
	cleanupQuitted chan struct{}
}

// UserInfo is the user information returned by the broker. We use that to build our own buckets content.
type UserInfo struct {
	Name  string
	UID   int
	Gecos string
	Dir   string
	Shell string

	Groups []GroupInfo
}

// GroupInfo is the group information returned by the broker. We use that to build our own buckets content.
type GroupInfo struct {
	Name string
	GID  int
}

// userDB is the struct stored in json format in the bucket.
type userDB struct {
	UserPasswdShadow

	// Additional entries
	LastLogin time.Time
}

func (u userDB) toUserPasswdShadow() UserPasswdShadow {
	return u.UserPasswdShadow
}

// groupDB is the struct stored in json format in the bucket.
type groupDB struct {
	Name string
	GID  int
}

// userToGroupsDB is the struct stored in json format to match uid to gids in the bucket.
type userToGroupsDB struct {
	UID  int
	GIDs []int
}

// groupToUsersDB is the struct stored in json format to match gid to uids in the bucket.
type groupToUsersDB struct {
	GID  int
	UIDs []int
}

type options struct {
	expirationDate  time.Time
	cleanOnNew      bool
	cleanupInterval time.Duration
	procDir         string // This is to force failure in tests.
}

// Option represents an optional function to override Cache default values.
type Option func(*options)

// WithExpirationDate overrides the default time for when a user should be cleaned from the cache.
func WithExpirationDate(date time.Time) Option {
	return func(o *options) {
		o.expirationDate = date
	}
}

// New creates a new database cache by creating or opening the underlying db.
func New(cacheDir string, args ...Option) (cache *Cache, err error) {
	dbPath := filepath.Join(cacheDir, dbName)
	defer decorate.OnError(&err, "could not create new database object at %q", dbPath)

	opts := options{
		expirationDate:  time.Now().Add(-1 * defaultEntryExpiration),
		cleanupInterval: defaultCleanupInterval,
		cleanOnNew:      true,
		procDir:         "/proc/",
	}
	for _, arg := range args {
		arg(&opts)
	}

	dirtyFlagPath := filepath.Join(cacheDir, dirtyFlagDbName)

	var db *bbolt.DB
	var i int
	for {
		db, err = openAndInitDB(dbPath, dirtyFlagPath)
		if err == nil {
			break
		}

		if errors.Is(err, shouldRetryDBError{}) {
			if i == 3 {
				return nil, errors.Unwrap(err)
			}
			i++
			continue
		}

		return nil, err
	}

	c := Cache{
		db:             db,
		dirtyFlagPath:  dirtyFlagPath,
		procDir:        opts.procDir,
		doClear:        make(chan struct{}),
		quit:           make(chan struct{}),
		cleanupQuitted: make(chan struct{}),
	}

	if opts.cleanOnNew {
		// Triggers a cleanup call when the db is opened
		if err := c.cleanExpiredUsers(opts.expirationDate); err != nil {
			slog.Warn(fmt.Sprintf("Could not clean database %v", err))
		}
	}

	cleanupRoutineStarted := make(chan struct{})
	go func() {
		defer close(c.cleanupQuitted)
		close(cleanupRoutineStarted)
		for {
			select {
			case <-c.doClear:
				func() {
					c.mu.Lock()
					defer c.mu.Unlock()

					if err := c.db.Close(); err != nil {
						slog.Warn(fmt.Sprintf("Could not close database %v", err))
					}

					db, err := openAndInitDB(dbPath, c.dirtyFlagPath)
					if err != nil {
						panic(fmt.Sprintf("CRITICAL: unrecoverable state: could not recreate database: %v", err))
					}
					c.db = db
				}()

			case <-time.After(opts.cleanupInterval):
				func() {
					c.mu.RLock()
					defer c.mu.RUnlock()

					slog.Debug("Starting scheduled cleaning of expired users")

					if err := c.cleanExpiredUsers(opts.expirationDate); err != nil {
						slog.Warn(fmt.Sprintf("Could not clean database %v", err))
					}
				}()

			case <-c.quit:
				return
			}
		}
	}()
	<-cleanupRoutineStarted

	return &c, nil
}

// openAndInitDB open a pre-existing database and potentially intializes its buckets.
// It clears up any database previously marked as dirty or if it’s corrupted.
func openAndInitDB(path, dirtyFlagPath string) (*bbolt.DB, error) {
	if _, err := os.Stat(dirtyFlagPath); err == nil {
		clearDatabase(path, dirtyFlagPath)
	}

	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		if errors.Is(err, bbolt.ErrInvalid) {
			clearDatabase(path, dirtyFlagPath)
			return nil, shouldRetryDBError{err: err}
		}
		return nil, fmt.Errorf("can't open database file: %v", err)
	}
	// Fail if permissions are not 0600
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("can't stat database file: %v", err)
	}
	perm := fileInfo.Mode().Perm()
	if perm != 0600 {
		return nil, fmt.Errorf("wrong file permission for %s: %o", path, perm)
	}

	// Create buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		var allBucketsNames []string
		for _, bucket := range allBuckets {
			allBucketsNames = append(allBucketsNames, string(bucket))
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}

		// Clear up any unknown buckets
		var bucketNamesToDelete [][]byte
		err = tx.ForEach(func(name []byte, b *bbolt.Bucket) error {
			if slices.Contains(allBucketsNames, string(name)) {
				return nil
			}
			bucketNamesToDelete = append(bucketNamesToDelete, name)
			return nil
		})
		if err != nil {
			return err
		}
		for _, bucketName := range bucketNamesToDelete {
			// We are in a RW transaction.
			_ = tx.DeleteBucket(bucketName)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return db, nil
}

// cleanExpiredUsers removes from the cache any user that exceeded the maximum amount of days without authentication.
func (c *Cache) cleanExpiredUsers(expirationDate time.Time) (err error) {
	defer decorate.OnError(&err, "could not clean up database")

	return c.db.Update(func(tx *bbolt.Tx) (err error) {
		// Cleans up the database
		activeUsers, err := getActiveUsers(c.procDir)
		if err != nil {
			return err
		}

		buckets, err := getAllBuckets(tx)
		if err != nil {
			return err
		}

		var expiredUsers []userDB
		// The foreach closure can't error out, so we can ignore the error.
		_ = buckets[userByIDBucketName].ForEach(func(k, v []byte) error {
			var u userDB
			if err := json.Unmarshal(v, &u); err != nil {
				slog.Warn(fmt.Sprintf("Could not unmarshal user %q: %v", string(k), err))
				return nil
			}

			if _, active := activeUsers[u.Name]; !active && u.LastLogin.Before(expirationDate) {
				expiredUsers = append(expiredUsers, u)
			}
			return nil
		})

		for _, u := range expiredUsers {
			slog.Debug(fmt.Sprintf("Deleting expired user %q", u.Name))
			if err := deleteUser(buckets, u.UID); err != nil {
				slog.Warn(fmt.Sprintf("Could not delete user %q: %v", u.Name, err))
			}
		}

		return nil
	})
}

// getActiveUsers walks through procDir and returns a map with the usernames of the owners of all active processes.
func getActiveUsers(procDir string) (activeUsers map[string]struct{}, err error) {
	defer decorate.OnError(&err, "could not get list of active users")

	activeUsers = make(map[string]struct{})

	dirEntries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, err
	}

	for _, dirEntry := range dirEntries {
		// Checks if the dirEntry represents a process dir (i.e. /proc/<pid>/)
		if _, err := strconv.Atoi(dirEntry.Name()); err != nil {
			continue
		}

		info, err := dirEntry.Info()
		if err != nil {
			return nil, err
		}

		stats, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, fmt.Errorf("could not get ownership of file %q", info.Name())
		}

		u, err := user.LookupId(strconv.Itoa(int(stats.Uid)))
		if err != nil {
			return nil, err
		}

		activeUsers[u.Name] = struct{}{}
	}
	return activeUsers, nil
}

// Close closes the db and signal the monitoring goroutine to stop.
func (c *Cache) Close() error {
	close(c.quit)
	<-c.cleanupQuitted
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.db.Close()
}

// requestClearDatabase ask for the clean goroutine to clear up the database.
// If we already have a pending request, do not block on it.
func (c *Cache) requestClearDatabase() {
	if err := os.WriteFile(c.dirtyFlagPath, nil, 0600); err != nil {
		slog.Warn(fmt.Sprintf("Could not write dirty file flag to signal clearing up the database: %v", err))
	}
	select {
	case c.doClear <- struct{}{}:
	case <-time.After(time.Millisecond): // Let the time for the cleanup goroutine for the initial start.
	}
}

func clearDatabase(dbPath, dirtyFlagPath string) {
	if err := os.Remove(dbPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Error(fmt.Sprintf("Could not delete %v to clear up cache: %v", dbPath, err))
	}
	if err := os.Remove(dirtyFlagPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Error(fmt.Sprintf("Could not delete %v to clear up dirty flag file: %v", dirtyFlagPath, err))
	}
}

// bucketWithName is a wrapper adding the name on top of a bbolt Bucket.
type bucketWithName struct {
	name string
	*bbolt.Bucket
}

// getAllBuckets returns all buckets that should be stored in the database.
func getAllBuckets(tx *bbolt.Tx) (map[string]bucketWithName, error) {
	buckets := make(map[string]bucketWithName)
	for _, name := range allBuckets {
		b := tx.Bucket(name)
		if b == nil {
			return nil, fmt.Errorf("bucket %v not found", name)
		}
		buckets[string(name)] = bucketWithName{name: string(name), Bucket: b}
	}

	return buckets, nil
}

// getBucket returns one bucket for a given name.
func getBucket(tx *bbolt.Tx, name string) (bucketWithName, error) {
	b := tx.Bucket([]byte(name))
	if b == nil {
		return bucketWithName{}, fmt.Errorf("bucket %v not found", name)
	}
	return bucketWithName{name: name, Bucket: b}, nil
}

// getFromBucket is a generic function to get any value of given type from a bucket. It returns an error if
// the returned value (json) could not be unmarshalled to the returned struct.
func getFromBucket[T any, K int | string](bucket bucketWithName, key K) (T, error) {
	// TODO: switch to https://github.com/golang/go/issues/45380 if accepted.
	var k []byte
	switch v := any(key).(type) {
	case int:
		k = []byte(strconv.Itoa(v))
	case string:
		k = []byte(v)
	default:
		panic(fmt.Sprintf("unhandled type: %T", key))
	}

	var r T

	data := bucket.Get(k)
	if data == nil {
		return r, NoDataFoundError{key: string(k), bucketName: bucket.name}
	}

	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("can't unmarshal bucket %q for key %v: %v", bucket.name, key, err)
	}

	return r, nil
}

// NoDataFoundError is returned when we didn’t find a matching entry.
type NoDataFoundError struct {
	key        string
	bucketName string
}

// Error implements the error interface to return key/bucket name.
func (err NoDataFoundError) Error() string {
	return fmt.Sprintf("no result matching %v in %v", err.key, err.bucketName)
}

// Is makes this error insensitive to the key and bucket name.
func (NoDataFoundError) Is(target error) bool { return target == NoDataFoundError{} }

// shouldRetryDBError is returned when we want to retry opening the database.
type shouldRetryDBError struct {
	err error
}

// Error implements the error interface.
func (err shouldRetryDBError) Error() string {
	return "ErrRetryDB"
}

// Unwrap allows to unwrap original error.
func (err shouldRetryDBError) Unwrap() error {
	return err.err
}

// Is makes this error insensitive to the key and bucket name.
func (shouldRetryDBError) Is(target error) bool { return target == shouldRetryDBError{} }
