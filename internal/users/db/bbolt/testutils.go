package bbolt

// All those functions and methods are only for tests.
// They are not exported, and guarded by testing assertions.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubuntu/authd/internal/testsdetection"
	"go.etcd.io/bbolt"
	"gopkg.in/yaml.v3"
)

const (
	// We need to replace the current time by a deterministic time in the golden files to be able to compare them.
	// We use the first second of the year 2020 as a recognizable value (which is not the zero value).
	redactedCurrentTime = "2020-01-01T00:00:00Z"

	// User homes can have an invalid prefix that we can adjust during tests.
	redactedUserHome = "@HOME_BASE@"
)

// Z_ForTests_CreateDBFromYAML creates the database inside destDir and loads the src file content into it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_CreateDBFromYAML(src, destDir string) (err error) {
	return Z_ForTests_CreateDBFromYAMLWithBaseHome(src, destDir, "")
}

// Z_ForTests_CreateDBFromYAMLWithBaseHome creates the database inside destDir and loads the src file content into it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_CreateDBFromYAMLWithBaseHome(src, destDir, baseHomeDir string) (err error) {
	testsdetection.MustBeTesting()

	src, err = filepath.Abs(src)
	if err != nil {
		return err
	}

	yamlData, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	dbPath := filepath.Join(destDir, dbFilename)
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := db.Close()
		if err == nil {
			err = closeErr
		}
	}()

	// load everything in a map.
	dbContent := make(map[string]map[string]string)
	if err := yaml.Unmarshal(yamlData, dbContent); err != nil {
		return err
	}

	// Create buckets and content.
	return db.Update(func(tx *bbolt.Tx) error {
		for bucketName, bucketContent := range dbContent {
			bucket, err := tx.CreateBucketIfNotExists([]byte(bucketName))
			if err != nil {
				return err
			}

			for key, val := range bucketContent {
				if bucketName == userByIDBucketName || bucketName == userByNameBucketName {
					// Replace the redacted time in the json value by a valid time.
					val = strings.Replace(val, redactedCurrentTime, time.Now().Format(time.RFC3339), 1)

					if baseHomeDir != "" {
						var u UserDB
						if err := json.Unmarshal([]byte(val), &u); err != nil {
							return err
						}

						u.Dir = strings.ReplaceAll(u.Dir, redactedUserHome, baseHomeDir)
						if err := os.MkdirAll(u.Dir, 0700); err != nil {
							return err
						}

						v, err := json.Marshal(u)
						if err != nil {
							return err
						}
						val = string(v)
					}
				}

				if err := bucket.Put([]byte(key), []byte(val)); err != nil {
					panic("programming error: put called in a RO transaction")
				}
			}
		}
		return nil
	})
}
