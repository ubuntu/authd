package main_test

import (
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

func buildCPAMModule(t *testing.T, sources []string, pkgConfigDeps []string, cFlags []string, soname string) string {
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
		"-Werror",
		"-g3",
		"-O0",
		"-DAUTHD_TEST_MODULE=1",
	)
	if len(pkgConfigDeps) > 0 {
		cFlags = append(cFlags,
			getPkgConfigFlags(t, append([]string{"--cflags"}, pkgConfigDeps...))...)
	}
	cmd.Args = append(cmd.Args, cFlags...)

	if testutils.IsAsan() {
		cmd.Args = append(cmd.Args, "-fsanitize=address,undefined")
	}
	// FIXME: This leads to an EOM error when loading the compiled module:
	// if testutils.IsRace() {
	// 	cmd.Args = append(cmd.Args, "-fsanitize=thread")
	// }
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

	if testutils.CoverDirForTests() != "" {
		cmd.Args = append(cmd.Args, "--coverage")
		cmd.Args = append(cmd.Args, "-fprofile-abs-path")

		notesFilename := soname + ".so-module.gcno"
		dataFilename := soname + ".so-module.gcda"

		libDir := filepath.Dir(libPath)
		gcovDir := filepath.Join(testutils.CoverDirForTests(), t.Name()+".gcov")
		err := os.MkdirAll(gcovDir, 0700)
		require.NoError(t, err, "TearDown: Impossible to create path %q", gcovDir)

		t.Cleanup(func() {
			t.Log("Running gcov...")
			gcov := exec.Command("gcov")
			gcov.Args = append(gcov.Args,
				"-pb", "-o", libDir,
				notesFilename)
			gcov.Dir = gcovDir
			out, err := gcov.CombinedOutput()
			require.NoError(t, err,
				"Teardown: Can't get coverage report on C library: %s", out)
			if string(out) != "" {
				t.Log(string(out))
			}

			// Also keep track of notes and data files as they're useful to generate
			// an html output locally using geninfo + genhtml.
			err = os.Rename(filepath.Join(libDir, dataFilename),
				filepath.Join(gcovDir, dataFilename))
			require.NoError(t, err,
				"Teardown: Can't move coverage report data for c Library: %v", err)
			err = os.Rename(filepath.Join(libDir, notesFilename),
				filepath.Join(gcovDir, notesFilename))
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

type actionArgsMap = map[pam_test.Action][]string

func createServiceFile(t *testing.T, name string, libPath string, args []string) string {
	t.Helper()

	return createServiceFileWithActionArgs(t, name, libPath, actionArgsMap{
		pam_test.Auth:     args,
		pam_test.Account:  args,
		pam_test.Password: args,
		pam_test.Session:  args,
	})
}

func createServiceFileWithActionArgs(t *testing.T, name string, libPath string, actionArgs actionArgsMap) string {
	t.Helper()

	serviceFile, err := pam_test.CreateService(t.TempDir(), name, []pam_test.ServiceLine{
		{Action: pam_test.Auth, Control: pam_test.SufficientRequisite, Module: libPath, Args: actionArgs[pam_test.Auth]},
		{Action: pam_test.Auth, Control: pam_test.Requisite, Module: pam_test.Ignore.String()},
		{Action: pam_test.Account, Control: pam_test.SufficientRequisite, Module: libPath, Args: actionArgs[pam_test.Account]},
		{Action: pam_test.Account, Control: pam_test.Requisite, Module: pam_test.Ignore.String()},
		{Action: pam_test.Password, Control: pam_test.SufficientRequisite, Module: libPath, Args: actionArgs[pam_test.Password]},
		{Action: pam_test.Password, Control: pam_test.Requisite, Module: pam_test.Ignore.String()},
		{Action: pam_test.Session, Control: pam_test.SufficientRequisite, Module: libPath, Args: actionArgs[pam_test.Session]},
		{Action: pam_test.Session, Control: pam_test.Requisite, Module: pam_test.Ignore.String()},
	})
	require.NoError(t, err, "Setup: Can't create service file %s", serviceFile)
	t.Logf("Created service file at %s", serviceFile)

	return serviceFile
}
