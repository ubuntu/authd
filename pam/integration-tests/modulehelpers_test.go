package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

func getPkgConfigFlags(t *testing.T, args []string) []string {
	t.Helper()

	out, err := exec.Command("pkg-config", args...).CombinedOutput()
	require.NoError(t, err, "Can't run pkg-config: %s", out)
	return strings.Split(strings.TrimSpace(string(out)), " ")
}

func buildCPAMModule(t *testing.T, sources []string, pkgConfigDeps []string, soname string) string {
	t.Helper()

	compiler := os.Getenv("CC")
	if compiler == "" {
		compiler = "cc"
	}

	//nolint:gosec // G204 it's a test so we should allow using any compiler safely.
	cmd := exec.Command(compiler)
	cmd.Dir = testutils.ProjectRoot()
	libPath := filepath.Join(t.TempDir(), soname+".so")

	require.NoError(t, os.MkdirAll(filepath.Dir(libPath), 0700),
		"Setup: Can't create loader build path")
	t.Logf("Compiling PAM Wrapper library at %s", libPath)
	cmd.Args = append(cmd.Args, "-o", libPath)
	cmd.Args = append(cmd.Args, sources...)
	cmd.Args = append(cmd.Args,
		"-Wall",
		"-g3",
		"-O0",
		"-DAUTHD_TEST_MODULE=1",
	)
	if len(pkgConfigDeps) > 0 {
		cmd.Args = append(cmd.Args,
			getPkgConfigFlags(t, append([]string{"--cflags"}, pkgConfigDeps...))...)
	}

	if modulesPath := os.Getenv("AUTHD_PAM_MODULES_PATH"); modulesPath != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("-DAUTHD_PAM_MODULES_PATH=%q",
			os.Getenv("AUTHD_PAM_MODULES_PATH")))
	}
	if pam_test.IsAddressSanitizerActive() {
		cmd.Args = append(cmd.Args, "-fsanitize=address,undefined")
	}
	if cflags := os.Getenv("CFLAGS"); cflags != "" && os.Getenv("DEB_BUILD_ARCH") == "" {
		cmd.Args = append(cmd.Args, strings.Split(cflags, " ")...)
	}

	cmd.Args = append(cmd.Args, []string{
		"-Wl,--as-needed",
		"-Wl,--allow-shlib-undefined",
		"-shared",
		"-fPIC",
		"-Wl,--unresolved-symbols=report-all",
		"-Wl,-soname," + soname + "",
		"-lpam",
	}...)
	if len(pkgConfigDeps) > 0 {
		cmd.Args = append(cmd.Args,
			getPkgConfigFlags(t, append([]string{"--libs"}, pkgConfigDeps...))...)
	}

	if ldflags := os.Getenv("LDFLAGS"); ldflags != "" && os.Getenv("DEB_BUILD_ARCH") == "" {
		cmd.Args = append(cmd.Args, strings.Split(ldflags, " ")...)
	}

	if testutils.CoverDir() != "" {
		cmd.Args = append(cmd.Args, "--coverage")
		cmd.Args = append(cmd.Args, "-fprofile-abs-path")

		notesFilename := soname + ".so-module.gcno"
		dataFilename := soname + ".so-module.gcda"

		libDir := filepath.Dir(libPath)

		t.Cleanup(func() {
			t.Log("Running gcov...")
			gcov := exec.Command("gcov")
			gcov.Args = append(gcov.Args,
				"-pb", "-o", libDir,
				notesFilename)
			gcov.Dir = testutils.CoverDir()
			out, err := gcov.CombinedOutput()
			require.NoError(t, err,
				"Teardown: Can't get coverage report on C library: %s", out)
			if string(out) != "" {
				t.Log(string(out))
			}

			// Also keep track of notes and data files as they're useful to generate
			// an html output locally using geninfo + genhtml.
			err = os.Rename(filepath.Join(libDir, dataFilename),
				filepath.Join(testutils.CoverDir(), dataFilename))
			require.NoError(t, err,
				"Teardown: Can't move coverage report data for c Library: %v", err)
			err = os.Rename(filepath.Join(libDir, notesFilename),
				filepath.Join(testutils.CoverDir(), notesFilename))
			require.NoError(t, err,
				"Teardown: Can't move coverage report notes for c Library: %v", err)
		})
	}

	t.Logf("Running compiler command: %s %s", cmd.Path, strings.Join(cmd.Args[1:], " "))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: could not compile PAM module %s: %s", soname, out)
	if string(out) != "" {
		t.Log(string(out))
	}

	return libPath
}

func createServiceFile(t *testing.T, name string, libPath string, args []string, ignoreError string) string {
	t.Helper()

	serviceFile := filepath.Join(t.TempDir(), name)
	t.Logf("Creating service file at %s", serviceFile)

	for idx, arg := range args {
		args[idx] = fmt.Sprintf("[%s]", strings.ReplaceAll(arg, "]", "\\]"))
	}

	err := os.WriteFile(serviceFile,
		[]byte(fmt.Sprintf(`auth [success=done ignore=ignore default=die] %[1]s %[2]s
auth requisite pam_debug.so auth=%[3]s
account [success=done ignore=ignore default=die] %[1]s %[2]s
account requisite pam_debug.so acct=%[3]s
session [success=done ignore=ignore default=die] %[1]s %[2]s
session requisite pam_debug.so acct=%[3]s
password [success=done ignore=ignore default=die] %[1]s %[2]s
password requisite pam_debug.so acct=%[3]s`,
			libPath, strings.Join(args, " "), ignoreError)),
		0600)
	require.NoError(t, err, "Setup: could not create service file")
	return serviceFile
}
