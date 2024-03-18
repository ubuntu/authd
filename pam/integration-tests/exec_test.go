package main_test

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

var execModuleSources = []string{"./pam/go-exec/module.c"}

const execServiceName = "exec-module"

func TestExecModule(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	if !pam.CheckPamHasStartConfdir() {
		t.Fatal("can't test with this libpam version!")
	}

	libPath := buildExecModule(t)
	execClient := buildExecClient(t)

	// We do multiple tests inside this test function not to have to re-compile
	// the library and to ensure that we don't have to care about merging its coverage.

	// These are the module initialization tests.
	moduleInitTests := map[string]struct {
		moduleArgs []string
		wantError  error
	}{
		// Error cases
		"Error on no arguments": {
			wantError: pam.ErrModuleUnknown,
		},
		"Error on empty executable parameter": {
			moduleArgs: []string{""},
			wantError:  pam.ErrModuleUnknown,
		},
		"Error on non existent executable parameter": {
			moduleArgs: []string{"/non-existent/file"},
			wantError:  pam.ErrModuleUnknown,
		},
		"Error on non executable parameter": {
			moduleArgs: execModuleSources,
			wantError:  pam.ErrModuleUnknown,
		},
		"Error on not runnable parameter": {
			moduleArgs: []string{filepath.Join(testutils.ProjectRoot())},
			wantError:  pam.ErrModuleUnknown,
		},
	}
	for name, tc := range moduleInitTests {
		t.Run("ModuleInit "+name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			tx := preparePamTransaction(t, libPath, "", tc.moduleArgs, "")
			require.ErrorIs(t, tx.Authenticate(0), tc.wantError)
		})
	}

	// The tests below are based on the ones of the pam_test.ModuleTransactionDummy
	// but we're using the exec commands to ensure that everything works as expected.
	// We don't split the tests in different functions not to to have to regenerate the
	// same library for each test and to ensure that the C coverage is properly computed.

	// These tests are meant to check the exec client behavior itself.
	cliTests := map[string]struct {
		methodCalls   []cliMethodCall
		rawModuleArgs []string
		wantError     error
	}{
		"SetGet Item": {
			methodCalls: []cliMethodCall{
				{m: "SetItem", args: []any{pam.Rhost, "some-rhost-value"}, r: []any{nil}},
				{m: "GetItem", args: []any{pam.Rhost}, r: []any{"some-rhost-value", nil}},
			},
		},
		"SetGet Item handling errors": {
			methodCalls: []cliMethodCall{
				{m: "SetItem", args: []any{pam.Item(-1), "some-value"}, r: []any{pam.ErrBadItem}},
				{m: "GetItem", args: []any{pam.Item(-1)}, r: []any{"", pam.ErrBadItem}},
			},
		},
		"SetGet Env": {
			methodCalls: []cliMethodCall{
				{m: "PutEnv", args: []any{"FooEnv=bar"}, r: []any{nil}},
				{m: "GetEnv", args: []any{"FooEnv"}, r: []any{"bar"}},
				{m: "GetEnv", args: []any{"AnotherEnv"}, r: []any{}},

				{m: "PutEnv", args: []any{"Bar=foo"}, r: []any{pam.Error(0)}},

				{m: "PutEnv", args: []any{"FooEnv="}},
				{m: "GetEnv", args: []any{"FooEnv"}, r: []any{}},

				{m: "PutEnv", args: []any{"FooEnv"}},
				{m: "GetEnv", args: []any{"FooEnv"}, r: []any{}},
			},
		},
		"SetGet Data": {
			methodCalls: []cliMethodCall{
				{m: "SetData", args: []any{"FooData", "bar"}, r: []any{nil}},
				{m: "GetData", args: []any{"FooData"}, r: []any{"bar", nil}},

				{m: "GetData", args: []any{"AnotherData"}, r: []any{nil, pam.ErrNoModuleData}},

				{m: "SetData", args: []any{"FooData", []int{1, 2, 3}}},
				{m: "GetData", args: []any{"FooData"}, r: []any{[]int{1, 2, 3}, nil}},

				{m: "SetData", args: []any{"FooData", nil}},
				{m: "GetData", args: []any{"FooData"}, r: []any{nil, pam.ErrNoModuleData}},
			},
		},
		"GetEnvList empty": {
			methodCalls: []cliMethodCall{
				{m: "GetEnvList", r: []any{map[string]string{}, nil}},
			},
		},
		"GetEnvList populated": {
			methodCalls: []cliMethodCall{
				{m: "PutEnv", args: []any{"Env=value"}},
				{m: "PutEnv", args: []any{"Env2=value2"}},
				{m: "GetEnvList", r: []any{
					map[string]string{
						"Env":  "value",
						"Env2": "value2",
					},
					nil,
				}},
			},
		},

		// Error cases
		"Error when not providing arguments": {
			rawModuleArgs: []string{"SetItem"},
			wantError:     pam_test.ErrInvalidArguments,
		},
		"Error when not providing no arguments": {
			rawModuleArgs: []string{"SetData|"},
			wantError:     pam_test.ErrInvalidArguments,
		},
		"Error when providing empty arguments": {
			methodCalls: []cliMethodCall{{m: "SetItem", args: []any{}}},
			wantError:   pam_test.ErrInvalidArguments,
		},
		"Error when not providing enough arguments": {
			methodCalls: []cliMethodCall{{m: "SetItem", args: []any{pam.User}}},
			wantError:   pam_test.ErrInvalidArguments,
		},
		"Error when providing empty return values": {
			methodCalls: []cliMethodCall{{m: "SetItem", args: []any{pam.User, "an-user"}, r: []any{}}},
			wantError:   pam_test.ErrReturnMismatch,
		},
		"Error when not providing enough return values": {
			methodCalls: []cliMethodCall{{m: "GetItem", args: []any{pam.User}, r: []any{}}},
			wantError:   pam_test.ErrReturnMismatch,
		},
		"Error when calling unknown method": {
			methodCalls: []cliMethodCall{{m: "ThisMethodDoesNotExist"}},
			wantError:   pam_test.ErrInvalidMethod,
		},
		"Error when argument types do not match arguments": {
			methodCalls: []cliMethodCall{{m: "SetItem", args: []any{"an-item", "value"}}},
			wantError:   pam_test.ErrArgumentTypeMismatch,
		},
		"Error when return values types do not match expected": {
			methodCalls: []cliMethodCall{
				{m: "GetItem", args: []any{pam.Item(-1)}, r: []any{"", "should have been an error"}},
			},
			wantError: pam_test.ErrReturnMismatch,
		},
		"Error when trying to compare an unexpected variant value": {
			methodCalls: []cliMethodCall{{m: "GetEnvList", r: []any{"", nil}}},
			wantError:   pam_test.ErrReturnMismatch,
		},
		"Error when trying to compare a not-matching variant value": {
			methodCalls: []cliMethodCall{{m: "GetEnvList", r: []any{"string", nil}}},
			wantError:   pam_test.ErrReturnMismatch,
		},
		"Error when getting not-available user data": {
			methodCalls: []cliMethodCall{{m: "GetData", args: []any{"NotAvailable"}}},
			wantError:   pam.ErrNoModuleData,
		},
	}
	for name, tc := range cliTests {
		t.Run("Client "+name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			if len(tc.rawModuleArgs) == 0 {
				tc.rawModuleArgs = methodCallsAsArgs(tc.methodCalls)
			}
			tx := preparePamTransaction(t, libPath, execClient, tc.rawModuleArgs, "")
			require.ErrorIs(t, tx.Authenticate(0), tc.wantError)
		})
	}

	// These tests are meant to check the exec client pam flags.
	actionFlags := map[string]struct {
		flags pam.Flags
	}{
		"No flags set":                    {},
		"Silent flag set":                 {flags: pam.Silent},
		"Silent and RefreshCred flag set": {flags: pam.Silent | pam.RefreshCred},
	}
	for name, tc := range actionFlags {
		t.Run("Flags "+name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			// FIXME: Do data-check per action.
			methodCalls := []cliMethodCall{
				{"GetData", []any{"exec-client-flags"}, []any{tc.flags, nil}},
			}

			tx := preparePamTransaction(t, libPath, execClient, methodCallsAsArgs(methodCalls), "")

			// We can't use performAllPAMActions since PAM adds some flags we don't control to SetCred and ChangeAuthTok
			t.Run("Authenticate", func(t *testing.T) { require.NoError(t, tx.Authenticate(tc.flags)) })
			t.Run("AcctMgmt", func(t *testing.T) { require.NoError(t, tx.AcctMgmt(tc.flags)) })
			t.Run("Open and Close Session", func(t *testing.T) {
				require.NoError(t, tx.OpenSession(tc.flags))
				require.NoError(t, tx.CloseSession(tc.flags))
			})
		})
	}

	// These tests are checking Get/Set item and ensuring those values are matching
	// both inside the client and in the calling application.
	itemsTests := map[string]struct {
		item  pam.Item
		value *string
		user  string

		wantValue    *string
		wantGetError error
		wantSetError error
	}{
		"Set user": {
			item:  pam.User,
			value: ptrValue("an user"),
		},
		"Returns empty when getting an unset user": {
			item:      pam.User,
			wantValue: ptrValue(""),
		},
		"Returns the user when getting a preset user": {
			item:      pam.User,
			user:      "preset PAM user",
			wantValue: ptrValue("preset PAM user"),
		},
		"Setting and getting an user": {
			item:      pam.User,
			value:     ptrValue("the-user"),
			wantValue: ptrValue("the-user"),
		},
		"Getting the preset service name": {
			item:      pam.Service,
			wantValue: ptrValue(execServiceName),
		},

		// Error cases
		"Error when setting invalid item": {
			item:         pam.Item(-1),
			value:        ptrValue("some value"),
			wantSetError: pam.ErrBadItem,
		},
		"Error when getting invalid item": {
			item:         pam.Item(-1),
			wantGetError: pam.ErrBadItem,
			wantValue:    ptrValue(""),
		},
	}
	for name, tc := range itemsTests {
		t.Run("Item "+name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			var methodCalls []cliMethodCall
			var wantExitError error

			if tc.value != nil {
				methodCalls = append(methodCalls,
					cliMethodCall{m: "SetItem", args: []any{tc.item, *tc.value}})
				wantExitError = tc.wantSetError
			}

			if tc.wantValue != nil {
				methodCalls = append(methodCalls,
					cliMethodCall{m: "GetItem", args: []any{tc.item}})
				wantExitError = tc.wantGetError
			}

			tx := preparePamTransaction(t, libPath, execClient, methodCallsAsArgs(methodCalls), tc.user)
			performAllPAMActions(t, tx, 0, wantExitError)

			if tc.value != nil && tc.wantSetError == nil {
				value, err := tx.GetItem(tc.item)
				require.Equal(t, *tc.value, value, "Item %v value mismatch", tc.item)
				require.NoError(t, err, "Can't get a PAM item %v", tc.item)
			}

			if tc.wantValue != nil && tc.wantGetError == nil {
				value, err := tx.GetItem(tc.item)
				require.Equal(t, *tc.wantValue, value, "Item %v value mismatch", tc.item)
				require.NoError(t, err, "Can't get a PAM item %v", tc.item)
			}
		})
	}

	// These tests are checking that setting and unsetting env variables works
	// both inside the executed module and the caller one.
	envTests := map[string]struct {
		env          string
		value        *string
		presetValues map[string]string
		skipPut      bool

		wantValue    *string
		wantPutError error
	}{
		"Put var": {
			env:   "AN_ENV",
			value: ptrValue("value"),
		},
		"Unset a not-previously set value": {
			env:          "NEVER_SET_ENV",
			wantPutError: pam.ErrBadItem,
			wantValue:    ptrValue(""),
		},
		"Unset a preset value": {
			presetValues: map[string]string{"PRESET_ENV": "hey!"},
			env:          "PRESET_ENV",
			wantValue:    ptrValue(""),
		},
		"Changes a preset var": {
			presetValues: map[string]string{"PRESET_ENV": "hey!"},
			env:          "PRESET_ENV",
			value:        ptrValue("hello!"),
			wantValue:    ptrValue("hello!"),
		},
		"Get an unset env": {
			skipPut:   true,
			env:       "AN_UNSET_ENV",
			wantValue: ptrValue(""),
		},
		"Gets an invalid env name": {
			env:       "",
			value:     ptrValue("Invalid Value"),
			wantValue: ptrValue(""),
			skipPut:   true,
		},

		// Error cases
		"Error when putting an invalid env name": {
			env:          "",
			value:        ptrValue("Invalid Value"),
			wantPutError: pam.ErrBadItem,
		},
	}
	for name, tc := range envTests {
		t.Run("Env "+name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			wantEnvList := map[string]string{}

			methodCalls := []cliMethodCall{
				{m: "GetEnvList", r: []any{map[string]string{}, nil}},
			}

			if tc.presetValues != nil && !tc.skipPut {
				for env, value := range tc.presetValues {
					methodCalls = append(methodCalls, cliMethodCall{
						m: "PutEnv", args: []any{fmt.Sprintf("%s=%s", env, value)}, r: []any{nil},
					})
				}
				wantEnvList = maps.Clone(tc.presetValues)
				methodCalls = append(methodCalls, cliMethodCall{
					m: "GetEnvList", r: []any{maps.Clone(wantEnvList), nil},
				})

				// TODO: Actually call another operation here with different arguments and
				// ensure that we set those env variables everywhere.
			}

			if !tc.skipPut {
				var env string
				if tc.value != nil {
					env = tc.env + "=" + *tc.value
				} else {
					env = tc.env
				}
				methodCalls = append(methodCalls, cliMethodCall{
					m: "PutEnv", args: []any{env}, r: []any{tc.wantPutError},
				})

				if tc.wantPutError == nil {
					if tc.value != nil {
						wantEnvList[tc.env] = *tc.value
					}
					if tc.value != nil && tc.wantValue != nil {
						wantEnvList[tc.env] = *tc.wantValue
					}
					if tc.value == nil {
						delete(wantEnvList, tc.env)
					}
				}
				methodCalls = append(methodCalls, cliMethodCall{
					m: "GetEnvList", r: []any{maps.Clone(wantEnvList), nil},
				})
			}

			if tc.wantValue != nil {
				methodCalls = append(methodCalls, cliMethodCall{
					m: "GetEnv", args: []any{tc.env}, r: []any{*tc.wantValue},
				})
			}

			tx := preparePamTransaction(t, libPath, execClient, methodCallsAsArgs(methodCalls), "")
			envList, err := tx.GetEnvList()
			require.NoError(t, err, "Setup: GetEnvList should not return an error")
			require.Len(t, envList, 0, "Setup: GetEnvList should have elements")

			require.NoError(t, tx.AcctMgmt(0), "Calling AcctMgmt should not error")

			gotEnv, err := tx.GetEnvList()
			require.NoError(t, err, "tx.GetEnvList should not return an error")
			require.Equal(t, wantEnvList, gotEnv, "returned env lits should match expected")
		})
	}

	// These tests are ensuring that data values can be set on a module and fetched during
	// various stages.
	dataTests := map[string]struct {
		key        string
		data       any
		presetData map[string]any
		skipSet    bool
		skipGet    bool

		wantData     any
		wantSetError error
		wantGetError error
	}{
		"Sets and gets data": {
			presetData: map[string]any{"some-data": []string{"hey! That's", "true"}},
			key:        "data",
			data:       []string{"hey! That's", "true"},
			wantData:   []string{"hey! That's", "true"},
		},
		"Set replaces data": {
			presetData: map[string]any{"some-data": []string{"hey! That's", "true"}},
			key:        "some-data",
			data: []map[string]string{
				{"hey": "yay"},
				{"foo": "bar"},
			},
			wantData: []map[string]string{
				{"hey": "yay"},
				{"foo": "bar"},
			},
		},

		// Error cases
		"Error when getting data that has never been set": {
			skipSet:      true,
			key:          "not set",
			wantGetError: pam.ErrNoModuleData,
		},
		"Error when getting data that has been removed": {
			presetData:   map[string]any{"some-data": []string{"hey! That's", "true"}},
			key:          "some-data",
			data:         nil,
			wantGetError: pam.ErrNoModuleData,
		},
	}
	for name, tc := range dataTests {
		t.Run("Data "+name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			var methodCalls []cliMethodCall

			if tc.presetData != nil && !tc.skipSet {
				for key, value := range tc.presetData {
					methodCalls = append(methodCalls, cliMethodCall{
						m: "SetData", args: []any{key, value},
					})
				}

				// TODO: Check those values are still valid for other action
			}

			if !tc.skipSet {
				methodCalls = append(methodCalls, cliMethodCall{
					"SetData", []any{tc.key, tc.data}, []any{tc.wantSetError},
				})
			}

			if !tc.skipGet {
				methodCalls = append(methodCalls, cliMethodCall{
					"GetData", []any{tc.key}, []any{tc.wantData, tc.wantGetError},
				})
			}

			tx := preparePamTransaction(t, libPath, execClient, methodCallsAsArgs(methodCalls), "")
			require.NoError(t, tx.Authenticate(0))
		})
	}
}

func preparePamTransaction(t *testing.T, libPath string, clientPath string, args []string, user string) *pam.Transaction {
	t.Helper()

	// libpam won't ever return a pam.ErrIgnore, so we use a fallback error.
	// We use incomplete here, but it could be any.
	const pamDebugIgnoreError = "incomplete"

	moduleArgs := []string{"--exec-debug"}
	if env := testutils.CoverDirEnv(); env != "" {
		moduleArgs = append(moduleArgs, "--exec-env", testutils.CoverDirEnv())
	}
	moduleArgs = append(moduleArgs, clientPath)

	serviceFile := createServiceFile(t, execServiceName, libPath, append(moduleArgs, args...),
		pamDebugIgnoreError)
	tx, err := pam.StartConfDir(filepath.Base(serviceFile), user, nil, filepath.Dir(serviceFile))
	require.NoError(t, err, "PAM: Error to initialize module")
	require.NotNil(t, tx, "PAM: Transaction is not set")
	t.Cleanup(func() { require.NoError(t, tx.End(), "PAM: can't end transaction") })

	return tx
}

func performAllPAMActions(t *testing.T, tx *pam.Transaction, flags pam.Flags, wantError error) {
	t.Helper()

	t.Run("Authenticate", func(t *testing.T) { require.ErrorIs(t, tx.Authenticate(flags), wantError) })
	t.Run("AcctMgmt", func(t *testing.T) { require.ErrorIs(t, tx.AcctMgmt(flags), wantError) })
	t.Run("ChangeAuthTok", func(t *testing.T) { require.ErrorIs(t, tx.ChangeAuthTok(flags), wantError) })
	t.Run("SetCred", func(t *testing.T) { require.ErrorIs(t, tx.SetCred(flags), wantError) })
	t.Run("Open and Close Session", func(t *testing.T) {
		require.ErrorIs(t, tx.OpenSession(flags), wantError)
		require.ErrorIs(t, tx.CloseSession(flags), wantError)
	})
}

func buildExecModule(t *testing.T) string {
	t.Helper()

	pkgConfigDeps := []string{"gio-2.0", "gio-unix-2.0"}
	return buildCPAMModule(t, execModuleSources, pkgConfigDeps,
		"pam_authd_exec"+strings.ToLower(t.Name()))
}

func buildExecClient(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "build", "-C", "cmd/exec-client")
	cmd.Dir = filepath.Join(testutils.CurrentDir())
	if testutils.CoverDir() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
	}
	if pam_test.IsAddressSanitizerActive() {
		// -asan is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-asan")
	}
	cmd.Args = append(cmd.Args, "-gcflags=-dwarflocationlists=true")
	cmd.Args = append(cmd.Args, "-tags=pam_tests_exec_client")
	cmd.Env = append(os.Environ(), `CGO_CFLAGS=-O0 -g3`)

	execPath := filepath.Join(t.TempDir(), "exec-client")
	t.Logf("Compiling Exec client at %s", execPath)
	t.Logf(strings.Join(cmd.Args, " "))

	cmd.Args = append(cmd.Args, "-o", execPath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: could not compile PAM exec client: %s", out)

	return execPath
}

func ptrValue[T any](value T) *T {
	return &value
}

type cliMethodCall struct {
	// m is the method name to call.
	m string
	// args is the arguments to pass to the method.
	args []any
	// r is the expected method return values
	r []any
}

func (cmc cliMethodCall) format() string {
	strMethodCall := cmc.m

	argsParser := func(values []any) string {
		var strValues []string
		for _, r := range values {
			strValues = append(strValues, getVariantString(r))
		}
		return strings.Join(strValues, ";")
	}

	strMethodCall += "|" + argsParser(cmc.args)

	if cmc.r != nil {
		strMethodCall += "|" + argsParser(cmc.r)
	}

	return strMethodCall
}

func getVariantString(value any) string {
	switch v := value.(type) {
	case pam.Error:
		return fmt.Sprint(int(v))
	case nil:
		return "<@mv nothing>"
	default:
		variant := dbus.MakeVariant(value)
		return dbus.MakeVariantWithSignature(variant, dbus.ParseSignatureMust("v")).String()
	}
}

func methodCallsAsArgs(methodCalls []cliMethodCall) []string {
	var args []string
	for _, mc := range methodCalls {
		args = append(args, mc.format())
	}
	return args
}
