package cache

import (
	"errors"

	"go.etcd.io/bbolt"
)

// BrokerForUser returns the broker ID assigned to the given username or an error if no entry was found.
func (c *Cache) BrokerForUser(username string) (brokerID string, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	u, err := c.UserByName(username)
	if err != nil {
		return "", err
	}

	err = c.db.View(func(tx *bbolt.Tx) error {
		bucket, err := getBucket(tx, userToBrokerBucketName)
		if err != nil {
			return errors.Join(ErrNeedsClearing, err)
		}

		brokerID, err = getFromBucket[string](bucket, u.UID)
		// The UserToBroker bucket is a string to string mapping, so there's no issues with marshalling.
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	return brokerID, nil
}
