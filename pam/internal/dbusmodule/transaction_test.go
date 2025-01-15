package dbusmodule_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/authd/pam/internal/dbusmodule"
)

const ifaceName = "com.ubuntu.authd.pam"
const objectPath = "/com/ubuntu/authd/pam"

func TestTransactionConnectionError(t *testing.T) {
	t.Parallel()

	tx, cleanup, err := dbusmodule.NewTransaction(context.TODO(), "invalid-address")
	require.Nil(t, tx, "Transaction must be unset")
	require.Nil(t, cleanup, "Cleanup func must be unset")
	require.NotNil(t, err, "Error must be set")
}

func TestTransactionHandler(t *testing.T) {
	t.Parallel()

	tx, _ := prepareTransaction(t, nil)
	dbusTx, ok := tx.(*dbusmodule.Transaction)
	require.True(t, ok, "Transaction should be a dbus module Transaction")
	require.ErrorIs(t, dbusTx.InvokeHandler(nil, 0, nil), pam.ErrAbort)
}

func TestTransactionSetEnv(t *testing.T) {
	t.Parallel()

	const setMethodName = "SetEnv"
	const unsetMethodName = "UnsetEnv"

	tests := map[string]struct {
		env            string
		methodReturns  methodReturn
		wantMethodCall methodCall
		wantError      error
	}{
		"Sets an env": {
			env:            "ENV=foo",
			methodReturns:  methodReturn{m: setMethodName, values: []any{0}},
			wantMethodCall: methodCall{setMethodName, []any{"ENV", "foo"}},
		},
		"Sets an empty env": {
			env:            "ENV=",
			methodReturns:  methodReturn{m: setMethodName, values: []any{0}},
			wantMethodCall: methodCall{setMethodName, []any{"ENV", ""}},
		},
		"Unsets an env": {
			env:            "ENV",
			methodReturns:  methodReturn{m: unsetMethodName, values: []any{0}},
			wantMethodCall: methodCall{unsetMethodName, []any{"ENV"}},
		},

		// Error cases
		"Errors when setting an env, receiving a DBus error": {
			env:       "ENV=foo",
			wantError: pam.ErrSystem,
		},
		"Errors when setting an env, receiving a PAM error": {
			env:            "ENV=foo",
			methodReturns:  methodReturn{m: setMethodName, values: []any{pam.ErrBadItem}},
			wantMethodCall: methodCall{setMethodName, []any{"ENV", "foo"}},
			wantError:      pam.ErrBadItem,
		},
		"Errors when unsetting an env, receiving a DBus error": {
			env:       "ENV",
			wantError: pam.ErrSystem,
		},
		"Errors when unsetting an env, receiving a PAM error": {
			env:            "ENV",
			methodReturns:  methodReturn{m: unsetMethodName, values: []any{pam.ErrAbort}},
			wantMethodCall: methodCall{unsetMethodName, []any{"ENV"}},
			wantError:      pam.ErrAbort,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tx, ts := prepareTransaction(t, []methodReturn{tc.methodReturns})

			err := tx.PutEnv(tc.env)
			if !errors.Is(tc.wantError, pam.ErrSystem) {
				calledMethods := ts.getCalledMethods()
				require.Len(t, calledMethods, 1, "Method calls not matching")
				require.Equal(t, tc.wantMethodCall, calledMethods[0], "Method calls mismatch")
			}
			requireDbusErrorIs(t, err, tc.wantError)
		})
	}
}

func TestTransactionGetEnv(t *testing.T) {
	t.Parallel()

	const methodName = "GetEnv"

	tests := map[string]struct {
		env            string
		methodReturns  methodReturn
		wantMethodCall methodCall
		wantValue      string
	}{
		"Gets an empty env": {
			env:            "ENV",
			wantMethodCall: methodCall{methodName, []any{"ENV"}},
			methodReturns:  methodReturn{m: methodName, values: []any{0, ""}},
		},
		"Gets an value env": {
			env:            "ENV",
			wantMethodCall: methodCall{methodName, []any{"ENV"}},
			methodReturns:  methodReturn{m: methodName, values: []any{0, "some-value"}},
			wantValue:      "some-value",
		},

		// Error cases
		"Errors when getting an env, receiving a DBus error": {
			env: "ENV",
		},
		"Errors when getting an env, receiving a PAM error": {
			env:            "ENV",
			wantMethodCall: methodCall{methodName, []any{"ENV", "foo"}},
			methodReturns:  methodReturn{m: methodName, values: []any{pam.ErrIncomplete, "some-value"}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tx, ts := prepareTransaction(t, []methodReturn{tc.methodReturns})
			require.NotNil(t, ts, "Setup: failed creating transaction")

			value := tx.GetEnv(tc.env)
			require.Equal(t, tc.wantValue, value, "Env value mismatch")
		})
	}
}

func TestTransactionGetEnvList(t *testing.T) {
	t.Parallel()

	const methodName = "GetEnvList"
	someEnvList := map[string]string{"env1": "value1", "env2": "value2"}

	tests := map[string]struct {
		methodReturns  methodReturn
		wantMethodCall methodCall
		wantEnvList    map[string]string
		wantError      error
	}{
		"Gets an empty env list": {
			wantMethodCall: methodCall{m: methodName},
			methodReturns:  methodReturn{m: methodName, values: []any{0, map[string]string{}}},
			wantEnvList:    map[string]string{},
		},
		"Gets a filled env list": {
			wantMethodCall: methodCall{m: methodName},
			methodReturns:  methodReturn{m: methodName, values: []any{0, someEnvList}},
			wantEnvList:    someEnvList,
		},

		// Error cases
		"Errors when getting an env list, receiving a DBus error": {
			wantError: pam.ErrSystem,
		},
		"Errors when getting an env, receiving a PAM error": {
			wantMethodCall: methodCall{m: methodName},
			methodReturns:  methodReturn{m: methodName, values: []any{pam.ErrBuf, someEnvList}},
			wantError:      pam.ErrBuf,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tx, ts := prepareTransaction(t, []methodReturn{tc.methodReturns})

			envList, err := tx.GetEnvList()
			require.Equal(t, tc.wantEnvList, envList, "Env list does not match")
			if !errors.Is(tc.wantError, pam.ErrSystem) {
				calledMethods := ts.getCalledMethods()
				require.Len(t, calledMethods, 1, "Method calls not matching")
				require.Equal(t, tc.wantMethodCall, calledMethods[0], "Method calls mismatch")
			}
			requireDbusErrorIs(t, err, tc.wantError)
		})
	}
}

func TestTransactionSetItem(t *testing.T) {
	t.Parallel()

	const methodName = "SetItem"

	tests := map[string]struct {
		item           pam.Item
		value          string
		methodReturns  methodReturn
		wantMethodCall methodCall
		wantError      error
	}{
		"Sets an item": {
			item:           pam.User,
			value:          "user",
			methodReturns:  methodReturn{m: methodName, values: []any{0}},
			wantMethodCall: methodCall{methodName, []any{pam.User, "user"}},
		},
		"Sets an empty item": {
			item:           pam.Rhost,
			value:          "",
			methodReturns:  methodReturn{m: methodName, values: []any{0}},
			wantMethodCall: methodCall{methodName, []any{pam.Rhost, ""}},
		},

		// Error cases
		"Errors when setting an item, receiving a DBus error": {
			value:     "item",
			wantError: pam.ErrSystem,
		},
		"Errors when setting an item, receiving a PAM error": {
			item:           pam.User,
			value:          "user",
			methodReturns:  methodReturn{m: methodName, values: []any{pam.ErrBadItem}},
			wantMethodCall: methodCall{methodName, []any{pam.User, "user"}},
			wantError:      pam.ErrBadItem,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tx, ts := prepareTransaction(t, []methodReturn{tc.methodReturns})

			err := tx.SetItem(tc.item, tc.value)
			if !errors.Is(tc.wantError, pam.ErrSystem) {
				calledMethods := ts.getCalledMethods()
				require.Len(t, calledMethods, 1, "Method calls not matching")
				require.Equal(t, tc.wantMethodCall, calledMethods[0], "Method calls mismatch")
			}
			requireDbusErrorIs(t, err, tc.wantError)
		})
	}
}

func TestTransactionGetItem(t *testing.T) {
	t.Parallel()

	const methodName = "GetItem"

	tests := map[string]struct {
		item           pam.Item
		methodReturns  methodReturn
		wantMethodCall methodCall
		wantValue      string
		wantError      error
	}{
		"Gets an item": {
			item:           pam.User,
			wantValue:      "user",
			methodReturns:  methodReturn{m: methodName, values: []any{0, "user"}},
			wantMethodCall: methodCall{methodName, []any{pam.User}},
		},
		"Gets an empty item": {
			item:           pam.Rhost,
			wantValue:      "",
			methodReturns:  methodReturn{m: methodName, values: []any{0, ""}},
			wantMethodCall: methodCall{methodName, []any{pam.Rhost}},
		},

		// Error cases
		"Errors when getting an item, receiving a DBus error": {
			item:      pam.Item(-1),
			wantError: pam.ErrSystem,
		},
		"Errors when getting an item, receiving a PAM error": {
			item:           pam.Item(-1),
			methodReturns:  methodReturn{m: methodName, values: []any{pam.ErrBadItem, "user"}},
			wantMethodCall: methodCall{methodName, []any{pam.Item(-1)}},
			wantError:      pam.ErrBadItem,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tx, ts := prepareTransaction(t, []methodReturn{tc.methodReturns})

			value, err := tx.GetItem(tc.item)
			require.Equal(t, tc.wantValue, value, "Item %v values do not match", tc.item)
			if !errors.Is(tc.wantError, pam.ErrSystem) {
				calledMethods := ts.getCalledMethods()
				require.Len(t, calledMethods, 1, "Method calls not matching")
				require.Equal(t, tc.wantMethodCall, calledMethods[0], "Method calls mismatch")
			}
			requireDbusErrorIs(t, err, tc.wantError)
		})
	}
}

func TestTransactionSetData(t *testing.T) {
	t.Parallel()

	const setMethodName = "SetData"
	const unsetMethodName = "UnsetData"
	testData := map[string]int32{"uno": 1, "due": 2, "tre": 3}
	variantTestData := dbus.MakeVariant(any(testData))

	tests := map[string]struct {
		key            string
		data           any
		methodReturns  methodReturn
		wantMethodCall methodCall
		wantError      error
	}{
		"Sets some data": {
			key:            "data",
			data:           testData,
			methodReturns:  methodReturn{m: setMethodName, values: []any{0}},
			wantMethodCall: methodCall{setMethodName, []any{"data", variantTestData}},
		},
		"Unsets some data": {
			key:            "data-to-unset",
			data:           nil,
			methodReturns:  methodReturn{m: unsetMethodName, values: []any{0}},
			wantMethodCall: methodCall{unsetMethodName, []any{"data-to-unset"}},
		},

		// Error cases
		"Errors when setting data, receiving a DBus error": {
			key:       "data",
			data:      testData,
			wantError: pam.ErrSystem,
		},
		"Errors when setting data, receiving a PAM error": {
			key:            "data",
			data:           testData,
			methodReturns:  methodReturn{m: setMethodName, values: []any{pam.ErrBuf}},
			wantMethodCall: methodCall{setMethodName, []any{"data", variantTestData}},
			wantError:      pam.ErrBuf,
		},
		"Errors when unsetting data, receiving a DBus error": {
			key:       "data",
			wantError: pam.ErrSystem,
		},
		"Errors when unsetting data, receiving a PAM error": {
			key:            "data-to-unset",
			data:           nil,
			methodReturns:  methodReturn{m: unsetMethodName, values: []any{pam.ErrAbort}},
			wantMethodCall: methodCall{unsetMethodName, []any{"data-to-unset"}},
			wantError:      pam.ErrAbort,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tx, ts := prepareTransaction(t, []methodReturn{tc.methodReturns})

			err := tx.SetData(tc.key, tc.data)
			if !errors.Is(tc.wantError, pam.ErrSystem) {
				calledMethods := ts.getCalledMethods()
				require.Len(t, calledMethods, 1, "Method calls not matching")
				require.Equal(t, tc.wantMethodCall, calledMethods[0], "Method calls mismatch")
			}
			requireDbusErrorIs(t, err, tc.wantError)
		})
	}
}

func TestTransactionGetData(t *testing.T) {
	t.Parallel()

	const methodName = "GetData"
	testData := map[string]int32{"uno": 1, "due": 2, "tre": 3}
	variantTestData := dbus.MakeVariant(any(testData))

	tests := map[string]struct {
		key            string
		methodReturns  methodReturn
		wantMethodCall methodCall
		wantData       any
		wantError      error
	}{
		"Gets some data": {
			key:            "data",
			wantData:       testData,
			methodReturns:  methodReturn{m: methodName, values: []any{0, variantTestData}},
			wantMethodCall: methodCall{methodName, []any{"data"}},
		},

		// Error cases
		"Errors when getting data, receiving a DBus error": {
			key:       "data",
			wantError: pam.ErrSystem,
		},
		"Errors when getting data, receiving a PAM error": {
			key:            "data",
			methodReturns:  methodReturn{m: methodName, values: []any{pam.ErrNoModuleData, variantTestData}},
			wantMethodCall: methodCall{methodName, []any{"data"}},
			wantError:      pam.ErrNoModuleData,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tx, ts := prepareTransaction(t, []methodReturn{tc.methodReturns})

			data, err := tx.GetData(tc.key)
			require.Equal(t, tc.wantData, data, "Data mismatch")
			if !errors.Is(tc.wantError, pam.ErrSystem) {
				calledMethods := ts.getCalledMethods()
				require.Len(t, calledMethods, 1, "Method calls not matching")
				require.Equal(t, tc.wantMethodCall, calledMethods[0], "Method calls mismatch")
			}
			requireDbusErrorIs(t, err, tc.wantError)
		})
	}
}

func TestStartStringConv(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		prompt                string
		promptFormat          string
		promptFormatArgs      []interface{}
		convStyle             pam.Style
		convError             pam.Error
		convShouldNotBeCalled bool

		want            string
		wantMethodCalls *methodCallExpectations
		wantError       error
	}{
		"Messages with error style are handled by conversation": {
			prompt:    "This is an error!",
			convStyle: pam.ErrorMsg,
			want:      "I'm handling it fine though",
		},
		"Conversation prompt can be formatted": {
			promptFormat:     "Sending some %s, right? %v",
			promptFormatArgs: []interface{}{"info", true},
			convStyle:        pam.TextInfo,
			want:             "And returning some text back",
		},

		// Error cases
		"Error if conversation receives a DBus error": {
			wantError:             pam.ErrSystem,
			convShouldNotBeCalled: true,
		},
		"Error if the conversation handler fails": {
			prompt:    "Tell me your secret!",
			convStyle: pam.PromptEchoOff,
			convError: pam.ErrBuf,
			wantError: pam.ErrBuf,
		},
		"Error when conversation uses binary content style": {
			prompt:                "I am a binary content\xff!",
			convStyle:             pam.BinaryPrompt,
			convError:             pam.ErrConv,
			wantError:             pam.ErrConv,
			convShouldNotBeCalled: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mce := methodCallExpectations{}
			prompt := tc.prompt
			if tc.promptFormat != "" {
				prompt = fmt.Sprintf(tc.promptFormat, tc.promptFormatArgs...)
			}

			if !tc.convShouldNotBeCalled {
				mce.add("Prompt", []any{tc.convStyle, prompt}, []any{tc.convError, tc.want})
			}

			tx, ts := prepareTransaction(t, mce.methodReturns)

			var reply pam.StringConvResponse
			var err error
			if tc.promptFormat != "" {
				reply, err = tx.StartStringConvf(tc.convStyle, tc.promptFormat,
					tc.promptFormatArgs...)
			} else {
				reply, err = tx.StartStringConv(tc.convStyle, tc.prompt)
			}

			if !errors.Is(tc.wantError, pam.ErrSystem) {
				require.Equal(t, mce.wantMethodCalls, ts.getCalledMethods(), "Method calls mismatch")
			}
			requireDbusErrorIs(t, err, tc.wantError)

			if tc.wantError != nil {
				require.Zero(t, reply)
				return
			}

			require.NotNil(t, reply)
			require.Equal(t, tc.want, reply.Response())
			require.Equal(t, tc.convStyle, reply.Style())
		})
	}
}

func TestTransactionGetUser(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		presetUser string
		getError   pam.Error
		convUser   string
		convError  pam.Error

		want      string
		wantError error
	}{
		"Getting a previously set user does not require conversation handler": {
			presetUser: "an-user",
			want:       "an-user",
		},
		"Getting a previously set user does not use conversation handler": {
			presetUser: "an-user",
			convUser:   "another-user",
			want:       "an-user",
		},
		"Getting the user uses conversation handler if none was set": {
			want:     "provided-user",
			convUser: "provided-user",
		},

		// Error cases
		"Error when can't get user item": {
			want:      "",
			getError:  pam.ErrBadItem,
			wantError: pam.ErrBadItem,
		},
		"Error when conversation fails": {
			want:      "",
			convError: pam.ErrConv,
			wantError: pam.ErrConv,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mce := methodCallExpectations{}
			prompt := "Who are you?"

			mce.add("GetItem", []any{pam.User}, []any{tc.getError, tc.presetUser})
			if tc.presetUser == "" && tc.getError == pam.Error(0) {
				mce.add("Prompt", []any{pam.PromptEchoOn, prompt}, []any{tc.convError, tc.convUser})
			}

			tx, ts := prepareTransaction(t, mce.methodReturns)
			user, err := tx.GetUser(prompt)
			require.Equal(t, tc.want, user, "User dos not mach")
			if !errors.Is(tc.wantError, pam.ErrSystem) {
				require.Equal(t, mce.wantMethodCalls, ts.getCalledMethods(), "Method calls mismatch")
			}
			requireDbusErrorIs(t, err, tc.wantError)
		})
	}
}

func TestStartBinaryConv(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		data      []byte
		wantError error
	}{
		// Error cases
		"Error as they are not supported": {
			wantError: pam.ErrConv,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tx, _ := prepareTransaction(t, nil)
			ret, err := tx.StartBinaryConv(tc.data)
			require.ErrorIs(t, err, tc.wantError)
			require.Nil(t, ret)
		})
	}
}

type methodCallExpectations struct {
	methodReturns   []methodReturn
	wantMethodCalls []methodCall
}

func (mce *methodCallExpectations) add(method string, args []any, ret []any) {
	mce.wantMethodCalls = append(mce.wantMethodCalls, methodCall{
		m: method, args: args,
	})
	mce.methodReturns = append(mce.methodReturns, methodReturn{
		m: method, values: ret,
	})
}

func requireDbusErrorIs(t *testing.T, err error, wantError error) {
	t.Helper()

	require.ErrorIs(t, err, wantError, "Error is not matching")

	if errors.Is(wantError, pam.ErrSystem) {
		var dbusError dbus.Error
		require.True(t, errors.As(err, &dbusError), "Error should be a dbus error")
	}
}

func prepareTransaction(t *testing.T, expectedReturns []methodReturn) (pam.ModuleTransaction, *testServer) {
	t.Helper()

	address, obj := prepareTestServer(t, expectedReturns)
	tx, cleanup, err := dbusmodule.NewTransaction(context.TODO(), address,
		dbusmodule.WithSharedConnection(true))
	require.NoError(t, err, "Setup: Can't connect to %s", address)
	t.Cleanup(cleanup)

	t.Logf("Using bus at address %s", address)

	return tx, obj
}

func prepareTestServer(t *testing.T, expectedReturns []methodReturn) (string, *testServer) {
	t.Helper()

	address, cleanup, err := testutils.StartBusMock()
	require.NoError(t, err, "Setup: Creating mock bus failed")
	t.Cleanup(cleanup)

	conn, err := dbus.Connect(address)
	require.NoError(t, err, "Setup: Connecting to system Bus failed")
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Logf("Failed closing the D-Bus connection: %v", err)
		}
	})

	obj := &testServer{t: t, mu: &sync.Mutex{}, returns: expectedReturns}
	err = conn.Export(obj, objectPath, ifaceName)
	require.NoError(t, err, "Setup: Exporting test server object to bus failed")

	reply, err := conn.RequestName(ifaceName, dbus.NameFlagDoNotQueue)
	require.NoError(t, err, "Setup: can't get dbus name")
	require.Equal(t, reply, dbus.RequestNameReplyPrimaryOwner,
		"Setup: can't get dbus name")

	return address, obj
}

func TestMain(m *testing.M) {
	log.SetLevel(log.DebugLevel)

	m.Run()
}
