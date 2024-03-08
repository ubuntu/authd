package pam_test

import (
	"fmt"
	"testing"

	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
)

func ptrValue[T any](value T) *T {
	return &value
}

func bytesPointerDecoder(ptr pam.BinaryPointer) ([]byte, error) {
	if ptr == nil {
		return nil, nil
	}
	return *(*[]byte)(ptr), nil
}

func TestSetGetItem(t *testing.T) {
	t.Parallel()
	t.Cleanup(MaybeDoLeakCheck)

	tests := map[string]struct {
		item  pam.Item
		value *string

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

		"Setting and getting an user": {
			item:      pam.User,
			value:     ptrValue("the-user"),
			wantValue: ptrValue("the-user"),
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(MaybeDoLeakCheck)

			tx := NewModuleTransactionDummy(nil)

			if tc.value != nil {
				err := tx.SetItem(tc.item, *tc.value)
				require.ErrorIs(t, err, tc.wantSetError)
			}

			if tc.wantValue != nil {
				value, err := tx.GetItem(tc.item)
				require.Equal(t, value, *tc.wantValue)
				require.ErrorIs(t, err, tc.wantGetError)
			}
		})
	}
}

func TestSetPutEnv(t *testing.T) {
	t.Parallel()
	t.Cleanup(MaybeDoLeakCheck)

	tests := map[string]struct {
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
			env:       "NEVER_SET_ENV",
			wantValue: ptrValue(""),
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(MaybeDoLeakCheck)

			tx := NewModuleTransactionDummy(nil)
			envList, err := tx.GetEnvList()
			require.NoErrorf(t, err, "Setup: GetEnvList should not return an error")
			require.Lenf(t, envList, 0, "Setup: GetEnvList should have elements")

			if tc.presetValues != nil && !tc.skipPut {
				for env, value := range tc.presetValues {
					err := tx.PutEnv(env + "=" + value)
					require.NoError(t, err)
				}
				envList, err = tx.GetEnvList()
				require.NoError(t, err)
				require.Equal(t, tc.presetValues, envList)
			}

			if !tc.skipPut {
				var env string
				if tc.value != nil {
					env = tc.env + "=" + *tc.value
				} else {
					env = tc.env
				}
				err := tx.PutEnv(env)
				require.ErrorIs(t, err, tc.wantPutError)

				wantEnv := map[string]string{}
				if tc.wantPutError == nil {
					if tc.value != nil {
						wantEnv = map[string]string{tc.env: *tc.value}
					}
					if tc.value != nil && tc.wantValue != nil {
						wantEnv = map[string]string{tc.env: *tc.wantValue}
					}
				}
				gotEnv, err := tx.GetEnvList()
				require.NoError(t, err, "tx.GetEnvList should not return an error")
				require.Equal(t, wantEnv, gotEnv, "returned env lits should match expected")
			}

			if tc.wantValue != nil {
				value := tx.GetEnv(tc.env)
				require.Equal(t, value, *tc.wantValue)
			}
		})
	}
}

func TestSetGetData(t *testing.T) {
	t.Parallel()
	t.Cleanup(MaybeDoLeakCheck)

	tests := map[string]struct {
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
			presetData: map[string]any{"some-data": []any{"hey! That's", true}},
			key:        "data",
			data:       []any{"hey! That's", true},
			wantData:   []any{"hey! That's", true},
		},

		"Set replaces data": {
			presetData: map[string]any{"some-data": []any{"hey! That's", true}},
			key:        "some-data",
			data: ModuleTransactionDummy{
				Items: map[pam.Item]string{pam.Tty: "yay"},
				Env:   map[string]string{"foo": "bar"},
			},
			wantData: ModuleTransactionDummy{
				Items: map[pam.Item]string{pam.Tty: "yay"},
				Env:   map[string]string{"foo": "bar"},
			},
		},

		// Error cases
		"Error when getting data that has never been set": {
			skipSet:      true,
			key:          "not set",
			wantGetError: pam.ErrNoModuleData,
		},

		"Error when getting data that has been removed": {
			presetData:   map[string]any{"some-data": []any{"hey! That's", true}},
			key:          "some-data",
			data:         nil,
			wantGetError: pam.ErrNoModuleData,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(MaybeDoLeakCheck)

			tx := NewModuleTransactionDummy(nil)

			if tc.presetData != nil && !tc.skipSet {
				for key, value := range tc.presetData {
					err := tx.SetData(key, value)
					require.NoError(t, err)
				}
			}

			if !tc.skipSet {
				err := tx.SetData(tc.key, tc.data)
				require.ErrorIs(t, err, tc.wantSetError)
			}

			if !tc.skipGet {
				data, err := tx.GetData(tc.key)
				require.ErrorIs(t, err, tc.wantGetError)
				require.Equal(t, tc.wantData, data)
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	t.Parallel()
	t.Cleanup(MaybeDoLeakCheck)

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
			t.Cleanup(MaybeDoLeakCheck)

			tx := NewModuleTransactionDummy(tc.convHandler)

			if tc.presetUser != "" {
				err := tx.SetItem(pam.User, tc.presetUser)
				require.NoError(t, err)
			}

			prompt := "Who are you?"
			user, err := tx.GetUser(prompt)
			require.ErrorIs(t, err, tc.wantError)
			require.Equal(t, tc.want, user)
		})
	}
}

func TestStartStringConv(t *testing.T) {
	t.Parallel()
	t.Cleanup(MaybeDoLeakCheck)

	tests := map[string]struct {
		prompt                string
		promptFormat          string
		promptFormatArgs      []interface{}
		convStyle             pam.Style
		convError             error
		convHandler           *pam.ConversationFunc
		convShouldNotBeCalled bool

		want      string
		wantError error
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
		"Error if no conversation handler is set": {
			convHandler: ptrValue(pam.ConversationFunc(nil)),
			wantError:   pam.ErrConv,
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
			t.Cleanup(MaybeDoLeakCheck)

			convFunCalled := false
			tx := NewModuleTransactionDummy(func() pam.ConversationFunc {
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
						return tc.want, tc.convError
					})
			}())

			var reply pam.StringConvResponse
			var err error

			if tc.promptFormat != "" {
				reply, err = tx.StartStringConvf(tc.convStyle, tc.promptFormat,
					tc.promptFormatArgs...)
			} else {
				reply, err = tx.StartStringConv(tc.convStyle, tc.prompt)
			}

			wantConFuncCalled := !tc.convShouldNotBeCalled && tc.convHandler == nil
			require.Equal(t, wantConFuncCalled, convFunCalled)
			require.ErrorIs(t, err, tc.wantError)

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

func TestStartBinaryConv(t *testing.T) {
	t.Parallel()
	t.Cleanup(MaybeDoLeakCheck)

	tests := map[string]struct {
		request     []byte
		convError   error
		convHandler *pam.ConversationHandler

		want      []byte
		wantError error
	}{
		"Simple binary conversation": {
			request: []byte{0x01, 0x02, 0x03},
			want:    []byte{0x00, 0x01, 0x02, 0x03, 0x4},
		},

		// Error cases
		"Error if no conversation handler is set": {
			convHandler: ptrValue(pam.ConversationHandler(nil)),
			wantError:   pam.ErrConv,
		},

		"Error if no binary conversation handler is set": {
			convHandler: ptrValue(pam.ConversationHandler(pam.ConversationFunc(
				func(s pam.Style, msg string) (string, error) {
					return "", nil
				}))),
			wantError: pam.ErrConv,
		},

		"Error if the conversation handler fails": {
			request:   []byte{0x03, 0x02, 0x01},
			convError: pam.ErrBuf,
			wantError: pam.ErrBuf,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(MaybeDoLeakCheck)

			convFunCalled := false
			tx := NewModuleTransactionDummy(func() pam.ConversationHandler {
				if tc.convHandler != nil {
					return *tc.convHandler
				}

				return pam.BinaryConversationFunc(
					func(ptr pam.BinaryPointer) ([]byte, error) {
						convFunCalled = true
						require.NotNil(t, ptr)
						bytes := *(*[]byte)(ptr)
						require.Equal(t, tc.request, bytes)
						return tc.want, tc.convError
					})
			}())

			response, err := tx.StartBinaryConv(tc.request)
			require.ErrorIs(t, err, tc.wantError)
			require.Equal(t, tc.convHandler == nil, convFunCalled)

			if tc.wantError != nil {
				require.Nil(t, response)
				return
			}

			defer response.Release()
			require.NotNil(t, response)
			require.Equal(t, pam.BinaryPrompt, response.Style())
			require.NotNil(t, response.Data())
			bytes, err := response.Decode(bytesPointerDecoder)
			require.NoError(t, err)
			require.Equal(t, tc.want, bytes)

			bytes, err = response.Decode(nil)
			require.ErrorContains(t, err, "nil decoder provided")
			require.Nil(t, bytes)
		})
	}
}

func TestStartBinaryPointerConv(t *testing.T) {
	t.Parallel()
	t.Cleanup(MaybeDoLeakCheck)

	tests := map[string]struct {
		request     []byte
		convError   error
		convHandler *pam.ConversationHandler

		want      []byte
		wantError error
	}{
		"With nil argument": {
			request: nil,
			want:    nil,
		},

		"With empty argument": {
			request: []byte{},
			want:    []byte{},
		},

		"With simple argument": {
			request: []byte{0x01, 0x02, 0x03},
			want:    []byte{0x00, 0x01, 0x02, 0x03, 0x4},
		},

		// Error cases
		"Error if no conversation handler is set": {
			convHandler: ptrValue(pam.ConversationHandler(nil)),
			wantError:   pam.ErrConv,
		},

		"Error if no binary conversation handler is set": {
			convHandler: ptrValue(pam.ConversationHandler(pam.ConversationFunc(
				func(s pam.Style, msg string) (string, error) {
					return "", nil
				}))),
			wantError: pam.ErrConv,
		},

		"Error if the conversation handler fails": {
			request:   []byte{0xde, 0xad, 0xbe, 0xef, 0xf},
			convError: pam.ErrBuf,
			wantError: pam.ErrBuf,
		},

		"Error if no conversation handler is set handles allocated data": {
			convError: pam.ErrSystem,
			want:      []byte{0xde, 0xad, 0xbe, 0xef, 0xf},
			wantError: pam.ErrSystem,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(MaybeDoLeakCheck)

			convFunCalled := false
			tx := NewModuleTransactionDummy(func() pam.ConversationHandler {
				if tc.convHandler != nil {
					return *tc.convHandler
				}

				return pam.BinaryPointerConversationFunc(
					func(ptr pam.BinaryPointer) (pam.BinaryPointer, error) {
						convFunCalled = true
						if tc.request == nil {
							require.Nil(t, ptr)
						} else {
							require.NotNil(t, ptr)
						}
						bytes := cBytesToBytes(ptr, len(tc.request))
						require.Equal(t, tc.request, bytes)
						return allocateCBytes(tc.want), tc.convError
					})
			}())
			res, err := tx.StartConv(pam.NewBinaryConvRequest(
				allocateCBytes(tc.request), releaseCBytesPointer))
			require.ErrorIs(t, err, tc.wantError)
			require.Equal(t, tc.convHandler == nil, convFunCalled)

			if tc.wantError != nil {
				require.Nil(t, res)
				return
			}

			response, ok := res.(pam.BinaryConvResponse)
			require.True(t, ok)
			defer response.Release()
			require.NotNil(t, response)
			require.Equal(t, pam.BinaryPrompt, response.Style())
			if tc.want == nil {
				require.Nil(t, response.Data())
			} else {
				require.NotNil(t, response.Data())
			}
			bytes, err := response.Decode(func(ptr pam.BinaryPointer) ([]byte, error) {
				return cBytesToBytes(ptr, len(tc.want)), nil
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, bytes)

			bytes, err = response.Decode(nil)
			require.ErrorContains(t, err, "nil decoder provided")
			require.Nil(t, bytes)
		})
	}
}

type multiConvHandler struct {
	t            *testing.T
	responses    []pam.ConvResponse
	wantRequests []pam.ConvRequest
	timesCalled  int
}

func (c *multiConvHandler) next() (pam.ConvRequest, pam.ConvResponse) {
	i := c.timesCalled
	c.timesCalled++

	return c.wantRequests[i], c.responses[i]
}

func (c *multiConvHandler) RespondPAM(style pam.Style, prompt string) (string, error) {
	wantReq, response := c.next()
	require.Equal(c.t, wantReq.Style(), style)
	stringReq, ok := wantReq.(pam.StringConvRequest)
	require.True(c.t, ok)
	require.Equal(c.t, stringReq.Prompt(), prompt)
	stringRes, ok := response.(pam.StringConvResponse)
	require.True(c.t, ok)
	return stringRes.Response(), nil
}

func (c *multiConvHandler) RespondPAMBinary(ptr pam.BinaryPointer) ([]byte, error) {
	wantReq, response := c.next()
	require.Equal(c.t, wantReq.Style(), pam.BinaryPrompt)

	binReq, ok := wantReq.(pam.BinaryConvRequester)
	require.True(c.t, ok)
	wantReqBytes, err := bytesPointerDecoder(binReq.Pointer())
	require.NoError(c.t, err)
	actualReqBytes, err := bytesPointerDecoder(ptr)
	require.NoError(c.t, err)
	require.Equal(c.t, wantReqBytes, actualReqBytes)

	bytes, err := response.(pam.BinaryConvResponse).Decode(bytesPointerDecoder)
	require.NoError(c.t, err)
	return bytes, nil
}

func TestStartConvMulti(t *testing.T) {
	t.Parallel()
	t.Cleanup(MaybeDoLeakCheck)

	tests := map[string]struct {
		requests []pam.ConvRequest

		wantResponses []pam.ConvResponse
		wantConvCalls *int
		wantError     error
	}{
		"Can address multiple string requests": {
			requests: []pam.ConvRequest{
				pam.NewStringConvRequest(pam.PromptEchoOff, "give some PromptEchoOff"),
				pam.NewStringConvRequest(pam.PromptEchoOn, "give some PromptEchoOn"),
				pam.NewStringConvRequest(pam.ErrorMsg, "give some ErrorMsg"),
				pam.NewStringConvRequest(pam.TextInfo, "give some TextInfo"),
			},
			wantResponses: []pam.ConvResponse{
				StringResponseDummy{pam.PromptEchoOff, "answer to PromptEchoOff"},
				StringResponseDummy{pam.PromptEchoOn, "answer to PromptEchoOn"},
				StringResponseDummy{pam.ErrorMsg, "answer to ErrorMsg"},
				StringResponseDummy{pam.TextInfo, "answer to TextInfo"},
			},
		},

		"Can address multiple binary requests": {
			requests: []pam.ConvRequest{
				NewBinaryRequestDummy(nil),
				NewBinaryRequestDummy(pam.BinaryPointer(&[]byte{})),
				NewBinaryRequestDummy(pam.BinaryPointer(&[]byte{0xFF, 0x00, 0xBA, 0xAB})),
				NewBinaryRequestDummy(pam.BinaryPointer(&[]byte{0x55})),
			},
			wantResponses: []pam.ConvResponse{
				&BinaryResponseDummy{pam.BinaryPointer(&[]byte{})},
				&BinaryResponseDummy{nil},
				&BinaryResponseDummy{pam.BinaryPointer(&[]byte{0x53})},
				&BinaryResponseDummy{pam.BinaryPointer(&[]byte{0xAF, 0x00, 0xBA, 0xAC})},
			},
		},

		"Can address multiple mixed binary and string requests ": {
			requests: []pam.ConvRequest{
				NewBinaryRequestDummy(nil),
				pam.NewStringConvRequest(pam.PromptEchoOff, "PromptEchoOff"),
				NewBinaryRequestDummy(pam.BinaryPointer(&[]byte{})),
				pam.NewStringConvRequest(pam.PromptEchoOn, "PromptEchoOn"),
				NewBinaryRequestDummy(pam.BinaryPointer(&[]byte{0xFF, 0x00, 0xBA, 0xAB})),
				pam.NewStringConvRequest(pam.ErrorMsg, "ErrorMsg"),
				NewBinaryRequestDummy(pam.BinaryPointer(&[]byte{0x55})),
				pam.NewStringConvRequest(pam.TextInfo, "TextInfo"),
			},
			wantResponses: []pam.ConvResponse{
				&BinaryResponseDummy{pam.BinaryPointer(&[]byte{})},
				StringResponseDummy{pam.PromptEchoOff, "PromptEchoOff"},
				&BinaryResponseDummy{pam.BinaryPointer(&[]byte{0x55})},
				StringResponseDummy{pam.PromptEchoOn, "PromptEchoOn"},
				&BinaryResponseDummy{nil},
				StringResponseDummy{pam.ErrorMsg, "ErrorMsg"},
				&BinaryResponseDummy{pam.BinaryPointer(&[]byte{0xAF, 0x00, 0xBA, 0xAC})},
				StringResponseDummy{pam.TextInfo, "TextInfo"},
			},
		},

		// Error cases
		"Error if no request is provided": {
			wantError: pam.ErrConv,
		},

		"Error if one of the multiple request fails": {
			requests: []pam.ConvRequest{
				NewBinaryRequestDummy(nil),
				pam.NewStringConvRequest(pam.PromptEchoOff, "PromptEchoOff"),
				NewBinaryRequestDummy(pam.BinaryPointer(&[]byte{})),
				pam.NewStringConvRequest(pam.PromptEchoOn, "PromptEchoOn"),
				NewBinaryRequestDummy(pam.BinaryPointer(&[]byte{0xFF, 0x00, 0xBA, 0xAB})),
				// The case below will lead to the whole request to fail!
				pam.NewStringConvRequest(pam.Style(-1), "Invalid style"),
				pam.NewStringConvRequest(pam.ErrorMsg, "ErrorMsg"),
				NewBinaryRequestDummy(pam.BinaryPointer(&[]byte{0x55})),
				pam.NewStringConvRequest(pam.TextInfo, "TextInfo"),
			},
			wantResponses: []pam.ConvResponse{
				&BinaryResponseDummy{pam.BinaryPointer(&[]byte{})},
				StringResponseDummy{pam.PromptEchoOff, "PromptEchoOff"},
				&BinaryResponseDummy{pam.BinaryPointer(&[]byte{0x55})},
				StringResponseDummy{pam.PromptEchoOn, "PromptEchoOn"},
				&BinaryResponseDummy{nil},
				StringResponseDummy{pam.Style(-1), "Invalid style"},
				StringResponseDummy{pam.ErrorMsg, "ErrorMsg"},
				&BinaryResponseDummy{pam.BinaryPointer(&[]byte{0xAF, 0x00, 0xBA, 0xAC})},
				StringResponseDummy{pam.TextInfo, "TextInfo"},
			},
			wantConvCalls: ptrValue(5),
			wantError:     pam.ErrConv,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(MaybeDoLeakCheck)

			require.Equalf(t, len(tc.wantResponses), len(tc.requests),
				"Setup: mismatch on expectations / requests numbers")

			convHandler := &multiConvHandler{
				t:            t,
				wantRequests: tc.requests,
				responses:    tc.wantResponses,
			}
			tx := NewModuleTransactionDummy(convHandler)

			responses, err := tx.StartConvMulti(tc.requests)
			require.ErrorIs(t, err, tc.wantError)

			wantConvCalls := len(tc.requests)
			if tc.wantConvCalls != nil {
				wantConvCalls = *tc.wantConvCalls
			}
			require.Equal(t, convHandler.timesCalled, wantConvCalls)

			if tc.wantError != nil {
				require.Nil(t, responses)
				return
			}

			require.NotNil(t, responses)
			require.Len(t, responses, len(tc.requests))

			for i, res := range responses {
				wantRes := tc.wantResponses[i]
				require.Equal(t, wantRes.Style(), res.Style())

				switch r := res.(type) {
				case pam.StringConvResponse:
					require.Equal(t, wantRes, res)
				case pam.BinaryConvResponse:
					wantBinRes, ok := wantRes.(pam.BinaryConvResponse)
					require.True(t, ok)
					wb, err := wantBinRes.Decode(bytesPointerDecoder)
					require.NoError(t, err)
					bytes, err := r.Decode(bytesPointerDecoder)
					require.NoError(t, err)
					require.Equal(t, wb, bytes)
				default:
					t.Fatalf("conversation %#v is not handled", r)
				}
			}
		})
	}
}
