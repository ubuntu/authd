package main_test

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

	libPath := buildExecModuleWithCFlags(t, []string{"-DAUTHD_TEST_EXEC_MODULE"})
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
			wantError:  pam.ErrSystem,
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
				{m: "GetEnv", args: []any{"AnotherEnv"}},

				{m: "PutEnv", args: []any{"Bar=foo"}, r: []any{pam.Error(0)}},

				{m: "PutEnv", args: []any{"FooEnv="}},
				{m: "GetEnv", args: []any{"FooEnv"}},

				{m: "PutEnv", args: []any{"FooEnv"}},
				{m: "GetEnv", args: []any{"FooEnv"}},
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
				{m: "GetData", args: []any{"FooData"}, r: []any{nil, nil}},
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
		"Error providing invalid variant argument": {
			rawModuleArgs: []string{"$not_A-variant Action"},
			wantError:     pam_test.ErrInvalidArguments,
		},
		"Error providing no action": {
			rawModuleArgs: []string{dbus.MakeVariant(map[string]dbus.Variant{}).String()},
			wantError:     pam_test.ErrInvalidMethod,
		},
		"Error providing invalid action type": {
			rawModuleArgs: []string{dbus.MakeVariant(
				map[string]dbus.Variant{"act": dbus.MakeVariant([]int{1, 2, 3})},
			).String()},
			wantError: pam_test.ErrInvalidMethod,
		},
		"Error when not providing arguments": {
			rawModuleArgs: []string{dbus.MakeVariant(
				map[string]dbus.Variant{"act": dbus.MakeVariant("SetItem")},
			).String()},
			wantError: pam_test.ErrInvalidArguments,
		},
		"Error when providing no arguments": {
			rawModuleArgs: []string{dbus.MakeVariant(
				map[string]dbus.Variant{
					"act":  dbus.MakeVariant("SetItem"),
					"args": dbus.MakeVariant([]dbus.Variant{}),
				},
			).String()},
			wantError: pam_test.ErrInvalidArguments,
		},
		"Error providing invalid arguments type": {
			rawModuleArgs: []string{dbus.MakeVariant(
				map[string]dbus.Variant{
					"act":  dbus.MakeVariant("GetItem"),
					"args": dbus.MakeVariant("not enough"),
				},
			).String()},
			wantError: pam_test.ErrInvalidArguments,
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
		"Error when client fails panicking": {
			methodCalls: []cliMethodCall{{m: "SimulateClientPanic", args: []any{"Client panicked! (As expected)"}}},
			wantError:   pam.ErrSymbol,
		},
		"Error when client fails because an unhandled error": {
			methodCalls: []cliMethodCall{{m: "SimulateClientError", args: []any{"Client error!"}}},
			wantError:   pam.ErrSystem,
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

			tx := preparePamTransactionWithActionArgs(t, libPath, execClient, actionArgsMap{
				pam_test.Auth: methodCallsAsArgs([]cliMethodCall{
					{"GetData", []any{"exec-client-flags-authenticate"}, []any{tc.flags, nil}},
				}),
				pam_test.Account: methodCallsAsArgs([]cliMethodCall{
					{"GetData", []any{"exec-client-flags-authenticate"}, []any{tc.flags, nil}},
				}),
				pam_test.Session: methodCallsAsArgs([]cliMethodCall{
					{"GetData", []any{"exec-client-flags-session"}, []any{tc.flags, nil}},
				}),
				// We can't fully test this since PAM adds some flags we don't control to SetCred and ChangeAuthTok
				pam_test.Password: methodCallsAsArgs([]cliMethodCall{}),
			}, "")

			performAllPAMActions(t, tx, tc.flags, nil)
		})
	}

	// These tests are meant to check the exec client arguments.
	actionArgs := map[string]struct {
		moduleArgs []string
	}{
		"Do not deadlock if invalid log path is provided": {
			[]string{"--exec-log", "/not-existent/file-path.log"},
		},
	}
	for name, tc := range actionArgs {
		t.Run("ActionArgs "+name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			moduleArgs := append(getModuleArgs(t, "", nil), tc.moduleArgs...)
			moduleArgs = append(moduleArgs, "--", execClient)
			serviceFile := createServiceFile(t, execServiceName, libPath, moduleArgs)
			tx := preparePamTransactionForServiceFile(t, serviceFile, "", nil)
			performAllPAMActions(t, tx, pam.Flags(0), nil)
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

			tx := preparePamTransactionWithActionArgs(t, libPath, execClient, actionArgsMap{
				pam_test.Auth: methodCallsAsArgs(methodCalls),
				pam_test.Account: methodCallsAsArgs([]cliMethodCall{
					{m: "GetEnvList", r: []any{maps.Clone(wantEnvList), nil}},
				}),
			}, "")
			envList, err := tx.GetEnvList()
			require.NoError(t, err, "Setup: GetEnvList should not return an error")
			require.Len(t, envList, 0, "Setup: GetEnvList should have elements")

			require.NoError(t, tx.Authenticate(0), "Calling AcctMgmt should not error")
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
		"Gets previously set data": {
			presetData: map[string]any{"some-old-data": []int{3, 2, 1}},
			key:        "some-old-data",
			skipSet:    true,
			wantData:   []int{3, 2, 1},
		},
		"Data can be nil": {
			// This is actually a libpam issue, but we should respect that for now
			// See: https://github.com/linux-pam/linux-pam/pull/780
			data:     nil,
			key:      "nil-data",
			wantData: nil,
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
		"No error when getting data that has been removed": {
			presetData: map[string]any{"some-data": []string{"hey! That's", "true"}},
			key:        "some-data",
			data:       nil,
			wantData:   nil,
		},

		// Error cases
		"Error when getting data that has never been set": {
			skipSet:      true,
			key:          "not set",
			wantGetError: pam.ErrNoModuleData,
		},
	}
	for name, tc := range dataTests {
		t.Run("Data "+name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			var presetMethodCalls []cliMethodCall
			var methodCalls []cliMethodCall
			var postMethodCalls []cliMethodCall

			if tc.presetData != nil {
				for key, value := range tc.presetData {
					presetMethodCalls = append(methodCalls, cliMethodCall{
						m: "SetData", args: []any{key, value},
					})
				}
			}

			if !tc.skipSet {
				methodCalls = append(methodCalls, cliMethodCall{
					"SetData", []any{tc.key, tc.data}, []any{tc.wantSetError},
				})
			}

			if !tc.skipGet {
				mc := cliMethodCall{
					"GetData", []any{tc.key}, []any{tc.wantData, tc.wantGetError},
				}
				methodCalls = append(methodCalls, mc)
				postMethodCalls = append(methodCalls, mc)
			}

			tx := preparePamTransactionWithActionArgs(t, libPath, execClient, actionArgsMap{
				pam_test.Auth:     methodCallsAsArgs(presetMethodCalls),
				pam_test.Account:  methodCallsAsArgs(methodCalls),
				pam_test.Password: methodCallsAsArgs(postMethodCalls),
			}, "")
			require.NoError(t, tx.Authenticate(0))
			require.NoError(t, tx.AcctMgmt(0))
			require.NoError(t, tx.ChangeAuthTok(0))
		})
	}

	// These tests are checking that string conversations are working as expected.
	stringConvTests := map[string]struct {
		prompt                string
		promptFormat          string
		promptFormatArgs      []interface{}
		convStyle             pam.Style
		convError             error
		convHandler           *pam.ConversationFunc
		convShouldNotBeCalled bool

		want           string
		stringResponse any
		wantError      error
		wantExitError  error
	}{
		"Messages with info style are handled by conversation": {
			prompt:    "This is an info message!",
			convStyle: pam.TextInfo,
		},
		"Messages with error style are handled by conversation": {
			prompt:    "This is an error message!",
			convStyle: pam.ErrorMsg,
		},
		"Messages with echo on style are handled by conversation": {
			prompt:    "This is an echo on message!",
			convStyle: pam.PromptEchoOn,
			want:      "I'm handling it perfectly!",
		},
		"Conversation prompt can be formatted": {
			promptFormat:     "Sending some %s, right? %v - But that's %v or %d?",
			promptFormatArgs: []interface{}{"info", true, nil, 123},
			convStyle:        pam.PromptEchoOff,
			want:             "And returning some text back",
		},

		// Error cases
		"Error if no conversation handler is set": {
			convHandler: ptrValue(pam.ConversationFunc(nil)),
			wantError:   pam.ErrConv,
		},
		"Error if the conversation handler fails": {
			prompt:    "Tell me your secret!",
			convStyle: pam.PromptEchoOff,
			convError: pam.ErrBuf,
			wantError: pam.ErrConv,
		},
		"Error when conversation uses binary content style": {
			prompt:                "I am a binary content\xff!",
			convStyle:             pam.BinaryPrompt,
			convError:             pam.ErrConv,
			wantError:             pam.ErrConv,
			convShouldNotBeCalled: true,
		},
		"Error when when parsing returned response fails": {
			prompt:         "Hello!",
			convStyle:      pam.PromptEchoOn,
			want:           "Hey, hey!",
			stringResponse: "Hey, hey!",
			wantExitError:  pam_test.ErrReturnMismatch,
		},
		"Error when when parsing returned value style fails": {
			prompt:    "Hello!",
			convStyle: pam.PromptEchoOn,
			want:      "Hey, hey!",
			stringResponse: map[string]dbus.Variant{
				"style": dbus.MakeVariant("shouldn't be a string"),
				"reply": dbus.MakeVariant("Hey, hey!"),
			},
			wantExitError: pam_test.ErrInvalidArguments,
		},
		"Error when when parsing returned reply fails": {
			prompt:    "Hello!",
			convStyle: pam.PromptEchoOff,
			want:      "Hey, hey!",
			stringResponse: map[string]dbus.Variant{
				"style": dbus.MakeVariant(pam.PromptEchoOff),
				"reply": dbus.MakeVariant(2.55),
			},
			wantExitError: pam_test.ErrInvalidArguments,
		},
	}
	for name, tc := range stringConvTests {
		t.Run("StringConv "+name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			convFunCalled := false
			convHandler := func() pam.ConversationFunc {
				if tc.convHandler != nil {
					return *tc.convHandler
				}
				prompt := tc.prompt
				if tc.promptFormat != "" {
					prompt = fmt.Sprintf(tc.promptFormat, tc.promptFormatArgs...)
				}
				return pam.ConversationFunc(
					func(style pam.Style, msg string) (string, error) {
						convFunCalled = true
						require.Equal(t, prompt, msg)
						require.Equal(t, tc.convStyle, style)
						switch style {
						case pam.PromptEchoOff, pam.PromptEchoOn:
							return tc.want, tc.convError
						default:
							return "", tc.convError
						}
					})
			}()

			var methodCalls []cliMethodCall
			wantStringResponse := any(nil)
			if tc.wantError == nil && tc.stringResponse == nil {
				wantStringResponse = map[string]dbus.Variant{
					"style": dbus.MakeVariant(tc.convStyle),
					"reply": dbus.MakeVariant(tc.want),
				}
			}
			if tc.stringResponse != nil {
				wantStringResponse = tc.stringResponse
			}

			wantReturnValues := []any{
				wantStringResponse,
				tc.wantError,
			}

			if tc.promptFormat != "" {
				methodCalls = append(methodCalls, cliMethodCall{
					m:    "StartStringConvf",
					args: append([]any{tc.convStyle, tc.promptFormat}, tc.promptFormatArgs...),
					r:    wantReturnValues,
				})
			} else {
				methodCalls = append(methodCalls, cliMethodCall{
					m:    "StartStringConv",
					args: []any{tc.convStyle, tc.prompt},
					r:    wantReturnValues,
				})
			}

			tx := preparePamTransactionWithConv(t, libPath, execClient,
				methodCallsAsArgs(methodCalls), "", convHandler)
			require.ErrorIs(t, tx.Authenticate(0), tc.wantExitError,
				"Authenticate does not return expected error")

			wantConFuncCalled := !tc.convShouldNotBeCalled && tc.convHandler == nil
			require.Equal(t, wantConFuncCalled, convFunCalled)
		})
	}

	// These tests are checking that GetUser works as expected, in case using conversation.
	tests := map[string]struct {
		presetUser  string
		convHandler pam.ConversationHandler

		want      string
		wantError error
	}{
		"Getting a previously set user does not require conversation handler": {
			presetUser: "an-user",
			want:       "an-user",
		},
		"Getting a previously set user does not use conversation handler": {
			presetUser: "an-user",
			want:       "an-user",
			convHandler: pam.ConversationFunc(func(s pam.Style, msg string) (string, error) {
				return "another-user", pam.ErrConv
			}),
		},
		"Getting the user uses conversation handler if none was set": {
			want: "provided-user",
			convHandler: pam.ConversationFunc(
				func(s pam.Style, msg string) (string, error) {
					require.Equal(t, msg, "Who are you?")
					if msg != "Who are you?" {
						return "", pam.ErrConv
					}
					if s == pam.PromptEchoOn {
						return "provided-user", nil
					}
					return "", pam.ErrConv
				}),
		},

		// Error cases
		"Error when no conversation is set": {
			want:      "",
			wantError: pam.ErrConv,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var methodCalls []cliMethodCall

			prompt := "Who are you?"
			methodCalls = append(methodCalls, cliMethodCall{
				m:    "GetUser",
				args: []any{prompt},
				r:    []any{tc.want, tc.wantError},
			})

			tx := preparePamTransactionWithConv(t, libPath, execClient,
				methodCallsAsArgs(methodCalls), tc.presetUser, tc.convHandler)
			require.NoError(t, tx.Authenticate(0), "Authenticate should not fail")
		})
	}
}

func TestExecModuleUnimplementedActions(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	if !pam.CheckPamHasStartConfdir() {
		t.Fatal("can't test with this libpam version!")
	}

	libPath := buildExecModuleWithCFlags(t, nil)
	execClient := buildExecClient(t)

	tx := preparePamTransaction(t, libPath, execClient, nil, "an-user")
	require.Error(t, tx.SetCred(pam.Flags(0)), pam.ErrIgnore)
	require.Error(t, tx.OpenSession(pam.Flags(0)), pam.ErrIgnore)
	require.Error(t, tx.CloseSession(pam.Flags(0)), pam.ErrIgnore)
}

func getModuleArgs(t *testing.T, clientPath string, args []string) []string {
	t.Helper()

	moduleArgs := []string{"--exec-debug"}
	if env := testutils.CoverDirEnv(); env != "" {
		moduleArgs = append(moduleArgs, "--exec-env", testutils.CoverDirEnv())
	}

	logFile := os.Stderr.Name()
	if !testutils.IsVerbose() {
		logFile = prepareFileLogging(t, "exec-module.log")
		saveArtifactsForDebug(t, []string{logFile})
	}
	moduleArgs = append(moduleArgs, "--exec-log", logFile)

	if clientPath != "" {
		moduleArgs = append(moduleArgs, "--", clientPath)
		moduleArgs = append(moduleArgs, "-client-log", logFile)
	}
	return append(moduleArgs, args...)
}

func preparePamTransaction(t *testing.T, libPath string, clientPath string, args []string, user string) *pam.Transaction {
	t.Helper()

	return preparePamTransactionWithConv(t, libPath, clientPath, args, user, nil)
}

func preparePamTransactionWithConv(t *testing.T, libPath string, clientPath string, args []string, user string, conv pam.ConversationHandler) *pam.Transaction {
	t.Helper()

	serviceFile := createServiceFile(t, execServiceName, libPath, getModuleArgs(t, clientPath, args))
	return preparePamTransactionForServiceFile(t, serviceFile, user, conv)
}

func preparePamTransactionWithActionArgs(t *testing.T, libPath string, clientPath string, actionArgs actionArgsMap, user string) *pam.Transaction {
	t.Helper()

	actionArgs = maps.Clone(actionArgs)
	for a := range actionArgs {
		actionArgs[a] = getModuleArgs(t, clientPath, actionArgs[a])
	}

	serviceFile := createServiceFileWithActionArgs(t, execServiceName, libPath, actionArgs)
	return preparePamTransactionForServiceFile(t, serviceFile, user, nil)
}

func preparePamTransactionForServiceFile(t *testing.T, serviceFile string, user string, conv pam.ConversationHandler) *pam.Transaction {
	t.Helper()

	var tx *pam.Transaction
	var err error

	// FIXME: pam.Transaction doesn't handle well pam.ConversationHandler(nil)
	if conv != nil && !reflect.ValueOf(conv).IsNil() {
		tx, err = pam.StartConfDir(filepath.Base(serviceFile), user, conv, filepath.Dir(serviceFile))
	} else {
		tx, err = pam.StartConfDir(filepath.Base(serviceFile), user, nil, filepath.Dir(serviceFile))
	}
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

	return buildExecModuleWithCFlags(t, nil)
}

func buildExecModuleWithCFlags(t *testing.T, cFlags []string) string {
	t.Helper()

	pkgConfigDeps := []string{"gio-2.0", "gio-unix-2.0"}
	// t.Name() can be a subtest, so replace the directory slash to get a valid filename.
	return buildCPAMModule(t, execModuleSources, pkgConfigDeps, cFlags,
		"pam_authd_exec"+strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_")))
}

func buildExecClient(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "build", "-C", "cmd/exec-client")
	cmd.Dir = filepath.Join(testutils.CurrentDir())
	if testutils.CoverDirForTests() != "" {
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
	t.Log(strings.Join(cmd.Args, " "))

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
	argsParser := func(values []any) []dbus.Variant {
		var variantValues []dbus.Variant
		for _, v := range values {
			variantValues = append(variantValues, getVariant(v))
		}
		return variantValues
	}

	callMap := map[string]dbus.Variant{}
	callMap["act"] = dbus.MakeVariant(cmc.m)
	callMap["args"] = dbus.MakeVariant(argsParser(cmc.args))

	if cmc.r != nil {
		callMap["exp"] = dbus.MakeVariant(argsParser(cmc.r))
	}

	return dbus.MakeVariant(callMap).String()
}

func getVariant(value any) dbus.Variant {
	switch v := value.(type) {
	case pam.Error:
		return getVariant(int(v))
	case nil:
		return getVariant("<@mv nothing>")
	default:
		return dbus.MakeVariant(value)
	}
}

func methodCallsAsArgs(methodCalls []cliMethodCall) []string {
	var args []string
	for _, mc := range methodCalls {
		args = append(args, mc.format())
	}
	return args
}
