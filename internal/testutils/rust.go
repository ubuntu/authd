package testutils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getCargoPath() (path string, isNightly bool, err error) {
	cargo := os.Getenv("CARGO_PATH")
	if cargo == "" {
		cargo = "cargo"
	}

	//nolint:gosec // G204 we define the parameters here.
	v, err := exec.Command(cargo, "--version").CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("cargo can't be executed: %w", err)
	}

	// Only nightly has code coverage enabled
	return cargo, strings.Contains(string(v), "nightly"), nil
}

// CanRunRustTests returns if we can run rust tests via cargo on this machine.
// It checks for code coverage report if supported.
func CanRunRustTests(coverageWanted bool) (err error) {
	_, isNightly, err := getCargoPath()
	if err != nil {
		return err
	}

	if !coverageWanted {
		return nil
	}

	// Only nightly has code coverage enabled
	if !isNightly {
		return errors.New("coverage is requested but your cargo/rust version does not support it (needs nightly)")
	}

	// We need grcov for coverage report. However, even --help or --version creates a profile file in current directory.
	// Doing that in a temporary directory we clean then.
	tmp, err := os.MkdirTemp("", "grcov-test-*")
	if err != nil {
		return fmt.Errorf("can't create temporary directory to test grcov: %w", err)
	}
	defer os.RemoveAll(tmp)

	cmd := exec.Command("grcov", "--version")
	cmd.Env = append(os.Environ(), "LLVM_PROFILE_FILE="+tmp)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("grcov is needed for coverage report and can't be executed: %w", err)
	}

	return nil
}

// BuildRustNSSLib builds the NSS library and links the compiled file to libPath.
func BuildRustNSSLib(t *testing.T, features ...string) (libPath string, rustCovEnv []string) {
	t.Helper()

	projectRoot := ProjectRoot()

	cargo, isNightly, err := getCargoPath()
	require.NoError(t, err, "Setup: looking for cargo")

	var target string
	rustDir := filepath.Join(projectRoot, "nss")
	rustCovEnv, target = trackRustCoverage(t, rustDir)

	features = append([]string{"integration_tests", "custom_socket"}, features...)

	// Builds the nss library.
	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.Command(cargo, "build", "--verbose",
		"--features", strings.Join(features, ","), "--target-dir", target)
	cmd.Env = append(os.Environ(), rustCovEnv...)
	cmd.Dir = projectRoot

	if isNightly && IsAsan() {
		cmd.Env = append(cmd.Env, "RUSTFLAGS=-Zsanitizer=address")
	}

	t.Log("Building NSS library...", cmd.Args)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: could not build Rust NSS library: %s", out)

	// When building the crate with dh-cargo, this env is set to indicate which architecture the code
	// is being compiled to. When it's set, the compiled is stored under target/$(DEB_HOST_RUST_TYPE)/debug,
	// rather than under target/debug, so we need to append at the end of target to ensure we use
	// the right path.
	// If the env is not set, the target stays the same.
	target = filepath.Join(target, os.Getenv("DEB_HOST_RUST_TYPE"))

	// Creates a symlink for the compiled library with the expected versioned name.
	libPath = filepath.Join(target, "libnss_authd.so.2")
	if err = os.Symlink(filepath.Join(target, "debug", "libnss_authd.so"), libPath); err != nil {
		require.ErrorIs(t, err, os.ErrExist, "Setup: failed to create versioned link to the library")
	}

	return libPath, rustCovEnv
}

// trackRustCoverage returns environment variables and target directory so that following commands
// runs with code coverage enabled.
// Note that for developping purposes and avoiding keeping building the rust program dependencies,
// TEST_RUST_TARGET environment variable can be set to an absolute path to keep iterative
// build artifacts.
// This then allow coverage to run in parallel, as each subprocess will have its own environment.
// You will need to call MergeCoverages() after m.Run().
// If code coverage is not enabled, it still returns an empty slice, but the target can be used.
func trackRustCoverage(t *testing.T, src string) (env []string, target string) {
	t.Helper()

	target = os.Getenv("TEST_RUST_TARGET")
	if target == "" {
		target = t.TempDir()
	}

	coverDir := CoverDirForTests()
	if coverDir == "" {
		return nil, target
	}
	coverDir = filepath.Join(coverDir, "rust-cov")

	t.Cleanup(func() {
		rustJSONCoverage := filepath.Join(coverDir, "rust_coverage.json")
		//nolint:gosec // G204 we define what we cover ourself
		cmd := exec.Command("grcov", coverDir,
			"--binary-path", filepath.Join(target, "debug"),
			"--source-dir", src,
			"--ignore-not-existing",
			"--ignore=**/build.rs",
			"--ignore=**/*_tests.rs",
			"--ignore=**/testutils/**",
			"-t", "covdir",
			"-o", rustJSONCoverage)
		cmd.Env = append(os.Environ(), "LLVM_PROFILE_FILE="+coverDir)

		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "Teardown: could not convert coverage to json format: %s", out)

		// Load our converted JSON profile.
		var results map[string]interface{}
		d, err := os.ReadFile(rustJSONCoverage)
		require.NoError(t, err, "Teardown: can't read our json coverage file")
		err = json.Unmarshal(d, &results)
		require.NoError(t, err, "Teardown: decode our json coverage file")

		// This is the destination file for rust coverage in go format.
		outF, err := os.Create(filepath.Join(coverDir, "rust2go_coverage"))
		require.NoErrorf(t, err, "Teardown: failed opening output golang compatible cover file: %s", err)
		defer func() { assert.NoError(t, outF.Close(), "Teardown: can’t close golang compatible cover file") }()

		// Scan our results to write to it.
		scan(t, results, fqdnToPath(t, src), outF)
	})

	return []string{
		"RUSTFLAGS=-C instrument-coverage",
		"LLVM_PROFILE_FILE=" + filepath.Join(coverDir, "rust-%p-%m.profraw"),
	}, target
}

// scan iterates over children files and folders elements recursively.
func scan(t *testing.T, results map[string]interface{}, p string, w io.Writer) {
	t.Helper()

	// Scan a file.
	r := results["coverage"]
	if r != nil {
		res, ok := r.([]interface{})
		if !ok {
			t.Fatalf("%v for coverage report is not a slice of floats in interface", r)
		}
		convertRustFileResult(t, res, p, w)
		return
	}

	// Scan children files or folders.
	r = results["children"]
	if r != nil {
		res, ok := r.(map[string]interface{})
		if !ok {
			t.Fatalf("children %v is not a map of data", r)
		}
		// Iterate over files or dir.
		for elem, subResults := range res {
			// We are not interesting in other code than ours
			if elem == "/" {
				continue
			}

			res, ok := subResults.(map[string]interface{})
			// Skip summary coverage-related data
			if !ok {
				continue
			}
			scan(t, res, filepath.Join(p, elem), w)
		}
	}
}

// convertRustFileResult converts rust-formatted coverage content to go one and writes it to w.
func convertRustFileResult(t *testing.T, results []interface{}, p string, w io.Writer) {
	t.Helper()

	for l, r := range results {
		v, ok := r.(float64)
		if !ok {
			t.Fatalf("%v for coverage report is not a float", r)
		}
		var covered string
		switch v {
		case -1:
			continue
		case 0:
			covered = "0"
		default:
			// We are in mode set, so don’t count the number of runs
			covered = "1"
		}
		// We are doing line coverage and we don’t have the source line handy. Set it to 9999 then.
		writeGoCoverageLine(t, w, p, l+1, 9999, covered)
	}
}
