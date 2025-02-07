package db

// All those functions and methods are only for tests.
// They are not exported, and guarded by testing assertions.

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testsdetection"
	"go.etcd.io/bbolt"
	"gopkg.in/yaml.v3"
)

// We need to replace the current time by a deterministic time in the golden files to be able to compare them.
// We use the first second of the year 2020 as a recognizable value (which is not the zero value).
var redactedCurrentTime = "2020-01-01T00:00:00Z"

// redactTime replace current time by a redacted version.
func redactTime(line string) string {
	testsdetection.MustBeTesting()

	re := regexp.MustCompile(`"LastLogin":"(.*?)"`)
	match := re.FindSubmatch([]byte(line))

	if len(match) <= 1 {
		// Not found
		return line
	}

	lastLogin := string(match[1])
	lastLoginTime, err := time.Parse(time.RFC3339, lastLogin)
	if err != nil {
		return line
	}
	if time.Since(lastLoginTime) < time.Minute*5 {
		return strings.Replace(line, lastLogin, redactedCurrentTime, 1)
	}

	return line
}

// Z_ForTests_DumpNormalizedYAML gets the content of the database, normalizes it
// (so that it can be compared with a golden file) and returns it as a YAML string.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_DumpNormalizedYAML(c *Database) (string, error) {
	testsdetection.MustBeTesting()

	d := make(map[string]map[string]string)

	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := c.db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, bucket *bbolt.Bucket) error {
			d[string(name)] = make(map[string]string)
			return bucket.ForEach(func(key, value []byte) error {
				d[string(name)][string(key)] = redactTime(string(value))
				return nil
			})
		})
	}); err != nil {
		return "", err
	}
	content, err := yaml.Marshal(d)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// Z_ForTests_FromYAML loads the content of the database from YAML.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_FromYAML(r io.Reader, destDir string) error {
	testsdetection.MustBeTesting()

	dbPath := filepath.Join(destDir, dbName)
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	d, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	// load everything in a map.
	dbContent := make(map[string]map[string]string)
	if err := yaml.Unmarshal(d, dbContent); err != nil {
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
				}
				if err := bucket.Put([]byte(key), []byte(val)); err != nil {
					panic("programming error: put called in a RO transaction")
				}
			}
		}
		return nil
	})
}

// Z_ForTests_CreateDBFromYAML creates the database inside destDir and loads the src file content into it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_CreateDBFromYAML(t *testing.T, src, destDir string) {
	t.Helper()
	testsdetection.MustBeTesting()

	if !path.IsAbs(src) {
		wd, err := os.Getwd()
		require.NoError(t, err, "Setup: should be able to get working directory")
		src = filepath.Join(wd, src)
	}

	f, err := os.Open(src)
	require.NoError(t, err, "Setup: should be able to read source file")
	defer f.Close()

	err = Z_ForTests_FromYAML(f, destDir)
	require.NoError(t, err, "Setup: should be able to write database file")
}

// Z_ForTests_DBName returns the name of the database.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_DBName() string {
	testsdetection.MustBeTesting()
	return dbName
}
