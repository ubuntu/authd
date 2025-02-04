package db

import (
	"errors"

	"go.etcd.io/bbolt"
)

// BrokerForUser returns the broker ID assigned to the given username, empty if it's not assigned yet
// or an error if no user was found in the database.
func (c *Database) BrokerForUser(username string) (brokerID string, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	u, err := c.UserByName(username)
	if err != nil {
		return "", err
	}

	err = c.db.View(func(tx *bbolt.Tx) error {
		bucket, err := getBucket(tx, userToBrokerBucketName)
		if err != nil {
			return err
		}

		brokerID, err = getFromBucket[string](bucket, u.UID)
		// Ignore the error if the user doesn't have an assigned broker yet.
		if err != nil && errors.Is(err, NoDataFoundError{}) {
			err = nil
		}
		return err
	})
	if err != nil {
		return "", err
	}

	return brokerID, nil
}
