package cache

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/ubuntu/decorate"
	"go.etcd.io/bbolt"
)

const (
	dbName = "authd.db"

	userByNameBucketName   = "UserByName"
	userByIDBucketName     = "UserByID"
	groupByNameBucketName  = "GroupByName"
	groupByIDBucketName    = "GroupByID"
	userToGroupsBucketName = "UserToGroups"
	groupToUsersBucketName = "GroupToUsers"
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
	mu sync.Mutex

	dirtyFlagPath string
	doClear       chan struct{}
	quit          chan struct{}
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

// New creates a new database cache by creating or opening the underlying db.
func New(cacheDir string) (cache *Cache, err error) {
	dbPath := filepath.Join(cacheDir, dbName)
	defer decorate.OnError(&err, "could not create new database object at %q", dbPath)

	dirtyFlagPath := dbPath + ".dirty"

	db, err := openAndInitDB(dbPath)
	if err != nil {
		return nil, err
	}

	c := Cache{
		db:            db,
		dirtyFlagPath: dirtyFlagPath,
		doClear:       make(chan struct{}),
		quit:          make(chan struct{}),
	}

	// TODO: clean up old users if not connected.
	go func() {
		for {
			select {
			case <-c.doClear:
				func() {
					c.mu.Lock()
					defer c.mu.Unlock()

					if err := c.db.Close(); err != nil {
						slog.Warn(fmt.Sprintf("Could not close database %v", err))
					}
					for err := os.RemoveAll(dbPath); err != nil; {
						slog.Error(fmt.Sprintf("Could not delete %v to clear up cache: %v", dbPath, err))
						time.Sleep(time.Second)
					}
					for err := os.RemoveAll(c.dirtyFlagPath); err != nil; {
						slog.Error(fmt.Sprintf("Could not delete %v to clear up cache: %v", c.dirtyFlagPath, err))
						time.Sleep(time.Second)
					}

					db, err := openAndInitDB(dbPath)
					if err != nil {
						panic(fmt.Sprintf("CRITICAL: unrecoverable state: could not recreate database: %v", err))
					}
					c.db = db
				}()
			case <-c.quit:
				return
			}
		}
	}()

	if _, err := os.Stat(dirtyFlagPath); err == nil {
		c.doClear <- struct{}{}
	}

	return &c, nil
}

// openAndInitDB open a pre-existing database and potentially intializes its buckets.
func openAndInitDB(path string) (*bbolt.DB, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}
	// Fail if permissions are not 0600
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	perm := fileInfo.Mode().Perm()
	if perm != 0600 {
		return nil, fmt.Errorf("wrong file permission for %s: %o", path, perm)
	}

	// Create buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, bucket := range allBuckets {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Close closes the db and signal the monitoring goroutine to stop.
func (c *Cache) Close() error {
	close(c.quit)
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
	default:
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
		return r, ErrNoDataFound{key: string(k), bucketName: bucket.name}
	}

	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("can't unmarshal bucket %q for key %v: %v", bucket.name, key, err)
	}

	return r, nil
}

// ErrNoDataFound is returned when we didn’t find a matching entry.
type ErrNoDataFound struct {
	key        string
	bucketName string
}

// Error implements the error interface to return key/bucket name.
func (err ErrNoDataFound) Error() string {
	return fmt.Sprintf("no result matching %v in %v", err.key, err.bucketName)
}

// Is makes this error insensitive to the key and bucket name.
func (ErrNoDataFound) Is(target error) bool { return target == ErrNoDataFound{} }
