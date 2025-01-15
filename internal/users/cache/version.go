package cache

import (
	"fmt"

	"go.etcd.io/bbolt"
)

const versionKey = "version"

// StoreVersion stores the provided version (which is expected to be the
// current authd version) in the database, so that we know in the future
// which authd version last updated the database.
func (c *Cache) StoreVersion(v string) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(versionBucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", versionBucketName)
		}
		return bucket.Put([]byte(versionKey), []byte(v))
	})
}

// GetVersion returns the authd version stored in the database (see StoreVersion).
func (c *Cache) GetVersion() (string, error) {
	var version string
	err := c.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(versionBucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", versionBucketName)
		}
		version = string(bucket.Get([]byte(versionKey)))
		return nil
	})
	return version, err
}
