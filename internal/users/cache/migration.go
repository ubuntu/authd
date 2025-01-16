package cache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ubuntu/authd/internal/semver"
	"github.com/ubuntu/authd/log"
	"go.etcd.io/bbolt"
)

const initialLowercaseUserAndGroupNamesVersion = "0.3.8"

func maybeMigrateToLowercaseUserAndGroupNames(c *Cache) error {
	// Get the current version.
	version, err := c.GetVersion()
	if err != nil {
		return fmt.Errorf("error getting version from database: %w", err)
	}

	if version != "" && !semver.IsValid(version) {
		log.Warningf(context.Background(), "Invalid version in database (%q), skipping migration", version)
		return nil
	}

	// Migrate if:
	// - the version is empty (database was created before we started storing the version)
	// - the version is less than the version where we started storing user and group names in lowercase
	if version == "" || semver.Compare(version, initialLowercaseUserAndGroupNamesVersion) < 0 {
		log.Infof(context.Background(), "Migrating database to lowercase user and group names (database version: %q)", version)
		if err := migrateToLowercaseUserAndGroupNames(c); err != nil {
			return err
		}
	}

	return nil
}

func migrateToLowercaseUserAndGroupNames(c *Cache) error {
	err := migrateToLowercaseUsernames(c)
	err2 := migrateToLowercaseGroups(c)
	return errors.Join(err, err2)
}

func migrateToLowercaseUsernames(c *Cache) error {
	allUsers, err := c.allUsers()
	if err != nil {
		return fmt.Errorf("error getting all users: %w", err)
	}

	for _, u := range allUsers {
		if u.Name == strings.ToLower(u.Name) {
			continue
		}

		err = migrateUserToLowercase(c, u)
		if err != nil {
			log.Warningf(context.Background(), "Error migrating user %q to lowercase: %v", u.Name, err)
		}
	}

	return nil
}

func migrateToLowercaseGroups(c *Cache) error {
	allGroups, err := c.allGroups()
	if err != nil {
		return fmt.Errorf("error getting all groups: %w", err)
	}

	for _, g := range allGroups {
		if g.Name == strings.ToLower(g.Name) {
			continue
		}

		err = migrateGroupToLowercase(c, g)
		if err != nil {
			log.Warningf(context.Background(), "Error migrating group %q to lowercase: %v", g.Name, err)
		}
	}

	return nil
}

func migrateGroupToLowercase(c *Cache, group groupDB) error {
	log.Debugf(context.Background(), "Migrating group %q to lowercase", group.Name)
	oldName := group.Name
	newName := strings.ToLower(group.Name)
	group.Name = newName

	// Update the group in all group buckets
	return c.db.Update(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return err
		}

		// Update the group in the GroupByID bucket.
		updateBucket(buckets[groupByIDBucketName], group.GID, group)

		// Update the group in the GroupByUGID bucket.
		if group.UGID != "" {
			updateBucket(buckets[groupByUGIDBucketName], group.UGID, group)
		}

		// Update the group in the GroupByName bucket.
		if err = buckets[groupByNameBucketName].Delete([]byte(oldName)); err != nil {
			return err
		}
		updateBucket(buckets[groupByNameBucketName], newName, group)

		return nil
	})
}

func migrateUserToLowercase(c *Cache, user userDB) error {
	log.Debugf(context.Background(), "Migrating user %q to lowercase", user.Name)
	oldName := user.Name
	newName := strings.ToLower(user.Name)
	user.Name = newName

	// Update the user in all user buckets
	err := c.db.Update(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return err
		}

		// Update the user in the UserByID bucket.
		updateBucket(buckets[userByIDBucketName], user.UID, user)

		// Update the user in the UserByName bucket.
		if err = buckets[userByNameBucketName].Delete([]byte(oldName)); err != nil {
			return err
		}
		updateBucket(buckets[userByNameBucketName], newName, user)

		return nil
	})
	if err != nil {
		return fmt.Errorf("error updating user %q: %w", user.Name, err)
	}

	// Rename the user in /etc/group
	// Note that we can't use gpasswd here because it checks for the existence of the user, which causes an NSS request
	// being sent to authd, but authd is not ready yet because we are still migrating the database.
	content, err := os.ReadFile("/etc/group")
	if err != nil {
		return fmt.Errorf("error reading /etc/group: %w", err)
	}
	content = bytes.ReplaceAll(content, []byte(oldName), []byte(newName))
	//nolint:gosec // G306 the /etc/group file should have 0644 permissions
	err = os.WriteFile("/etc/group", content, 0644)
	if err != nil {
		return fmt.Errorf("error writing /etc/group: %w", err)
	}

	return nil
}
