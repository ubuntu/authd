package gdm

import (
	"testing"

	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

func TestSendToGdm(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		value []byte

		wantReturn                   []byte
		wantError                    error
		wantConvHandlerNotToBeCalled bool
	}{
		"JSON null data can be sent and received": {
			value: []byte(`null`),
		},
		"JSON number can be sent and received": {
			value: []byte(`1.5`),
		},
		"Single char is sent and received as string": {
			value: []byte(`"m"`),
		},
		"JSON null is returned": {
			value:      []byte(`"give me 🚫"`),
			wantReturn: []byte("null"),
		},
		"Utf-8 data is sent and returned": {
			value:      []byte(`"give me 🍕"`),
			wantReturn: []byte(`"😋"`),
		},

		// Error cases
		"Error on empty data": {
			value:                        []byte{},
			wantError:                    ErrInvalidJSON,
			wantConvHandlerNotToBeCalled: true,
		},
		"Error on nil data": {
			value:                        nil,
			wantError:                    ErrInvalidJSON,
			wantConvHandlerNotToBeCalled: true,
		},
		"Error with empty data returned": {
			value:      []byte(`"give me 🗑‼"`),
			wantReturn: []byte{},
			wantError:  ErrInvalidJSON,
		},
		"Error with nil data returned": {
			value:      []byte(`"give me 🚫"`),
			wantReturn: []byte(nil),
			wantError:  ErrInvalidJSON,
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			convFuncCalled := false
			mt := pam_test.NewModuleTransactionDummy(pam.BinaryPointerConversationFunc(
				func(ptr pam.BinaryPointer) (pam.BinaryPointer, error) {
					convFuncCalled = true
					require.NotNil(t, ptr)
					req, err := decodeJSONProtoMessage(ptr)
					require.NoError(t, err)
					require.Equal(t, tc.value, req)
					if tc.wantReturn != nil {
						msg, err := newJSONProtoMessage(tc.wantReturn)
						return pam.BinaryPointer(msg), err
					}
					msg, err := newJSONProtoMessage(req)
					return pam.BinaryPointer(msg), err
				}))

			data, err := sendToGdm(mt, tc.value)
			require.Equal(t, convFuncCalled, !tc.wantConvHandlerNotToBeCalled)

			if tc.wantError != nil {
				require.Error(t, tc.wantError, err)
				return
			}
			require.NoError(t, err)

			if tc.wantReturn != nil {
				require.Equal(t, tc.wantReturn, data)
				return
			}

			require.Equal(t, tc.value, data)
		})
	}
}
