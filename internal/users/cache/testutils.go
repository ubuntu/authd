package cache

// All those functions and methods are only for tests.
// They are not exported, and guarded by testing assertions.

import (
	"io"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ubuntu/authd/internal/testsdetection"
	"go.etcd.io/bbolt"
	"gopkg.in/yaml.v3"
)

//nolint:unused // This is used for tests, with methods that are using go linking. Not part of exported API.
var redactedTimes = map[string]string{
	"AAAAATIME": "2004-10-20T11:06:23Z",
	"BBBBBTIME": "2006-06-01T10:08:04Z",
	"CCCCCTIME": "2010-01-11T08:05:34Z",
	"DDDDDTIME": "2010-10-10T10:10:00Z",
	"EEEEETIME": "2011-04-28T14:30:85Z",
	"ABCDETIME": "now",
}

// redactTime replace current time by a redacted version.
//
//nolint:unused // This is used for tests, with methods that are using go linking. Not part of exported API.
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

// dumpToYaml deserializes the cache database as a string in yaml format.
//
//nolint:unused // This is used for tests, with go linking. Not part of exported API.
func (c *Cache) dumpToYaml() (string, error) {
	testsdetection.MustBeTesting()

	d := make(map[string]map[string]string)

	c.mu.RLock()
	defer c.mu.RUnlock()

	username := "root"
	currentUser, err := user.Current()
	if err != nil {
		return "", err
	}
	if currentUser.Name != "" {
		username = currentUser.Name
	}

	if err := c.db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, bucket *bbolt.Bucket) error {
			d[string(name)] = make(map[string]string)
			return bucket.ForEach(func(key, value []byte) error {
				key = []byte(strings.Replace(string(key), username, "ACTIVE_USER", 1))
				value = []byte(strings.ReplaceAll(string(value), username, "ACTIVE_USER"))

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

// dbfromYAML loads a yaml formatted of the buckets from a reader and dump it into destDir, with its dbname.
//
//nolint:unused // This is used for tests, with go linking. Not part of exported API.
func dbfromYAML(r io.Reader, destDir string) error {
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

	username := "root"
	currentUser, err := user.Current()
	if err != nil {
		return err
	}
	if currentUser.Name != "" {
		username = currentUser.Name
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
					// Replace ACTIVE_USER name by the uid of an active user.
					val = strings.ReplaceAll(val, "ACTIVE_USER", username)
					key = strings.Replace(key, "ACTIVE_USER", username, 1)

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
