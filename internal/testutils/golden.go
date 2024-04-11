package testutils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	goldenPath string
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

// LoadWithUpdateFromGolden loads the element from a plaintext golden file.
// It will update the file if the update flag is used prior to loading it.
func LoadWithUpdateFromGolden(t *testing.T, data string, opts ...GoldenOption) string {
	t.Helper()

	o := goldenOptions{
		goldenPath: GoldenPath(t),
	}

	for _, opt := range opts {
		opt(&o)
	}

	if update {
		t.Logf("updating golden file %s", o.goldenPath)
		err := os.MkdirAll(filepath.Dir(o.goldenPath), 0750)
		require.NoError(t, err, "Cannot create directory for updating golden files")
		err = os.WriteFile(o.goldenPath, []byte(data), 0600)
		require.NoError(t, err, "Cannot write golden file")
	}

	want, err := os.ReadFile(o.goldenPath)
	require.NoError(t, err, "Cannot load golden file")

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
