package testutils

import (
	"context"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/log"
	"gopkg.in/yaml.v3"
)

var update bool

const (
	// UpdateGoldenFilesEnv is the environment variable used to indicate go test that
	// the golden files should be overwritten with the current test results.
	UpdateGoldenFilesEnv = `TESTS_UPDATE_GOLDEN`
)

func init() {
	if os.Getenv(UpdateGoldenFilesEnv) != "" {
		update = true
	}
}

type goldenOptions struct {
	goldenPath    string
	goldenTracker *GoldenTracker
}

// GoldenOption is a supported option reference to change the golden files comparison.
type GoldenOption func(*goldenOptions)

// WithGoldenPath overrides the default path for golden files used.
func WithGoldenPath(path string) GoldenOption {
	return func(o *goldenOptions) {
		if path != "" {
			o.goldenPath = path
		}
	}
}

// WithGoldenTracker sets the golden tracker to mark the golden as used.
func WithGoldenTracker(gt *GoldenTracker) GoldenOption {
	return func(o *goldenOptions) {
		if gt != nil {
			o.goldenTracker = gt
		}
	}
}

func parseOptions(t *testing.T, opts ...GoldenOption) goldenOptions {
	t.Helper()

	o := goldenOptions{
		goldenPath: GoldenPath(t),
	}

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

// LoadWithUpdateFromGolden loads the element from a plaintext golden file.
// It will update the file if the update flag is used prior to loading it.
func LoadWithUpdateFromGolden(t *testing.T, data string, opts ...GoldenOption) string {
	t.Helper()

	o := parseOptions(t, opts...)

	if update {
		t.Logf("updating golden file %s", o.goldenPath)
		err := os.MkdirAll(filepath.Dir(o.goldenPath), 0750)
		require.NoError(t, err, "Cannot create directory for updating golden files")
		err = os.WriteFile(o.goldenPath, []byte(data), 0600)
		require.NoError(t, err, "Cannot write golden file")
	}

	want, err := os.ReadFile(o.goldenPath)
	require.NoError(t, err, "Cannot load golden file")

	if o.goldenTracker != nil {
		o.goldenTracker.MarkUsed(t, WithGoldenPath(o.goldenPath))
	}

	return string(want)
}

// LoadWithUpdateFromGoldenYAML load the generic element from a YAML serialized golden file.
// It will update the file if the update flag is used prior to deserializing it.
func LoadWithUpdateFromGoldenYAML[E any](t *testing.T, got E, opts ...GoldenOption) E {
	t.Helper()

	t.Logf("Serializing object for golden file")
	data, err := yaml.Marshal(got)
	require.NoError(t, err, "Cannot serialize provided object")
	want := LoadWithUpdateFromGolden(t, string(data), opts...)

	var wantDeserialized E
	err = yaml.Unmarshal([]byte(want), &wantDeserialized)
	require.NoError(t, err, "Cannot create expanded policy objects from golden file")

	return wantDeserialized
}

// NormalizeName transforms name input with illegal characters replaced or removed.
func NormalizeName(t *testing.T, name string) string {
	t.Helper()

	name = strings.ReplaceAll(name, `\`, "_")
	name = strings.ReplaceAll(name, ":", "")
	name = strings.ToLower(name)
	return name
}

// TestFamilyPath returns the path of the dir for storing fixtures and other files related to the test.
func TestFamilyPath(t *testing.T) string {
	t.Helper()

	// Ensures that only the name of the parent test is used.
	super, _, _ := strings.Cut(t.Name(), "/")

	return filepath.Join("testdata", super)
}

// GoldenPath returns the golden path for the provided test.
func GoldenPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(TestFamilyPath(t), "golden")
	_, sub, found := strings.Cut(t.Name(), "/")
	if found {
		path = filepath.Join(path, NormalizeName(t, sub))
	}

	return path
}

// UpdateEnabled returns true if updating the golden files is requested.
func UpdateEnabled() bool {
	return update
}

// GoldenTracker is a structure to track used golden files in tests.
type GoldenTracker struct {
	mu   *sync.Mutex
	used map[string]struct{}
}

// NewGoldenTracker create a new [GoldenTracker] that checks if golden files are used.
func NewGoldenTracker(t *testing.T) GoldenTracker {
	t.Helper()

	gt := GoldenTracker{
		mu:   &sync.Mutex{},
		used: make(map[string]struct{}),
	}

	require.False(t, strings.Contains(t.Name(), "/"),
		"Setup: %T should be used from a parent test, %s is not", gt, t.Name())

	if slices.ContainsFunc(RunningTests(), func(r string) bool {
		prefix := t.Name() + "/"
		return strings.HasPrefix(r, prefix) && len(r) > len(prefix)
	}) {
		t.Logf("%T disabled, can't work on partial tests", gt)
		return gt
	}

	t.Cleanup(func() {
		if t.Failed() {
			return
		}

		goldenPath := GoldenPath(t)

		var entries []string
		err := filepath.WalkDir(goldenPath, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				log.Errorf(context.TODO(), "TearDown: Reading test golden files %s: %v", path, err)
				t.FailNow()
			}
			if path == goldenPath {
				return nil
			}
			entries = append(entries, path)
			return nil
		})
		if err != nil {
			log.Errorf(context.TODO(), "TearDown: Walking test golden files %s: %v", goldenPath, err)
			t.FailNow()
		}

		gt.mu.Lock()
		defer gt.mu.Unlock()

		t.Log("Checking golden files in", goldenPath)
		var unused []string
		for _, e := range entries {
			if _, ok := gt.used[e]; ok {
				continue
			}
			unused = append(unused, e)
		}
		if len(unused) > 0 {
			log.Errorf(context.TODO(), "TearDown: Unused golden files have been found:\n  %#v\n  known are %#v",
				unused, slices.Collect(maps.Keys(gt.used)))
			t.FailNow()
		}
	})

	return gt
}

// MarkUsed marks a golden file as being used.
func (gt *GoldenTracker) MarkUsed(t *testing.T, opts ...GoldenOption) {
	t.Helper()

	gt.mu.Lock()
	defer gt.mu.Unlock()

	o := parseOptions(t, opts...)
	require.Nil(t, o.goldenTracker, "Setup: GoldenTracker option is not supported")
	gt.used[o.goldenPath] = struct{}{}

	basePath := filepath.Dir(o.goldenPath)
	if basePath == GoldenPath(t) {
		gt.used[basePath] = struct{}{}
	}
}
