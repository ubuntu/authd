package cache

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
)

// UserPasswdShadow is the struct representing an user ready for nss requests.
type UserPasswdShadow struct {
	Name  string
	UID   int
	GID   int
	Gecos string // Gecos is an optional field. It can be empty.
	Dir   string
	Shell string

	// Shadow entries
	LastPwdChange  int
	MaxPwdAge      int
	PwdWarnPeriod  int
	PwdInactivity  int
	MinPwdAge      int
	ExpirationDate int
}

// UserByID returns a user matching this uid or an error if the database is corrupted or no entry was found.
// Upon corruption, clearing the database is requested.
func (c *Cache) UserByID(uid int) (UserPasswdShadow, error) {
	u, err := getUser(c, userByIDBucketName, uid)
	return u.toUserPasswdShadow(), err
}

// UserByName returns a user matching this name or an error if the database is corrupted or no entry was found.
// Upon corruption, clearing the database is requested.
func (c *Cache) UserByName(name string) (UserPasswdShadow, error) {
	u, err := getUser(c, userByNameBucketName, name)
	return u.toUserPasswdShadow(), err
}

// AllUsers returns all users or an error if the database is corrupted.
// Upon corruption, clearing the database is requested.
func (c *Cache) AllUsers() (all []UserPasswdShadow, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	err = c.db.View(func(tx *bbolt.Tx) error {
		bucket, err := getBucket(tx, userByIDBucketName)
		if err != nil {
			return err
		}

		return bucket.ForEach(func(key, value []byte) error {
			var e userDB
			if err := json.Unmarshal(value, &e); err != nil {
				return fmt.Errorf("can't unmarshal user in bucket %q for key %v: %v", userByIDBucketName, key, err)
			}
			all = append(all, e.toUserPasswdShadow())
			return nil
		})
	})

	if err != nil {
		c.requestClearDatabase()
		return nil, err
	}

	return all, nil
}

// getUser returns an user matching the key or an error if the database is corrupted or no entry was found.
// Upon corruption, clearing the database is requested.
func getUser[K int | string](c *Cache, bucketName string, key K) (u userDB, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	err = c.db.View(func(tx *bbolt.Tx) error {
		bucket, err := getBucket(tx, bucketName)
		if err != nil {
			c.requestClearDatabase()
			return err
		}

		u, err = getFromBucket[userDB](bucket, key)
		if err != nil {
			if !errors.Is(err, NoDataFoundError{}) {
				c.requestClearDatabase()
			}
			return err
		}

		return nil
	})

	if err != nil {
		return userDB{}, err
	}

	return u, nil
}
