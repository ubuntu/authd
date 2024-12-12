package cache

// All those functions and methods are only for tests.
// They are not exported, and guarded by testing assertions.

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testsdetection"
	"go.etcd.io/bbolt"
	"gopkg.in/yaml.v3"
)

var redactedTimes = map[string]string{
	"AAAAATIME": "2004-10-20T11:06:23Z",
	"BBBBBTIME": "2006-06-01T10:08:04Z",
	"CCCCCTIME": "2010-01-11T08:05:34Z",
	"DDDDDTIME": "2010-10-10T10:10:00Z",
	"EEEEETIME": "2011-04-28T14:30:85Z",
	"ABCDETIME": "now",
}

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
	var isNow bool
	if time.Since(lastLoginTime) < time.Minute*5 {
		isNow = true
	}

	for redacted, time := range redactedTimes {
		if lastLogin != time && (time != "now" || !isNow) {
			continue
		}

		return strings.Replace(line, lastLogin, redacted, 1)
	}
	return line
}

// Z_ForTests_DumpNormalizedYAML gets the content of the database, normalizes it
// (so that it can be compared with a golden file) and returns it as a YAML string.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_DumpNormalizedYAML(c *Cache) (string, error) {
	testsdetection.MustBeTesting()

	d := make(map[string]map[string]string)

	c.mu.RLock()
	defer c.mu.RUnlock()

	uid := os.Geteuid()

	if err := c.db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, bucket *bbolt.Bucket) error {
			d[string(name)] = make(map[string]string)
			return bucket.ForEach(func(key, value []byte) error {
				key = []byte(strings.Replace(string(key), strconv.Itoa(uid), "{{CURRENT_UID}}", 1))
				value = []byte(strings.ReplaceAll(string(value), strconv.Itoa(uid), "{{CURRENT_UID}}"))

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

	uid := os.Geteuid()

	// Create buckets and content.
	return db.Update(func(tx *bbolt.Tx) error {
		for bucketName, bucketContent := range dbContent {
			bucket, err := tx.CreateBucketIfNotExists([]byte(bucketName))
			if err != nil {
				return err
			}

			for key, val := range bucketContent {
				if bucketName == userByIDBucketName || bucketName == userByNameBucketName {
					// Replace {{CURRENT_UID}} with the UID of the current process
					val = strings.ReplaceAll(val, "{{CURRENT_UID}}", strconv.Itoa(uid))
					key = strings.Replace(key, "{{CURRENT_UID}}", strconv.Itoa(uid), 1)

					// Replace the redacted time in the json value by a valid time.
					for redacted, t := range redactedTimes {
						if t == "now" {
							t = time.Now().Format(time.RFC3339)
						}
						val = strings.Replace(val, redacted, t, 1)
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

// Z_ForTests_CreateDBFromYAML creates the database inside destDir and loads the src file content into it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_CreateDBFromYAML(t *testing.T, src, destDir string) {
	t.Helper()
	testsdetection.MustBeTesting()

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
