package gdm

import (
	"slices"
	"testing"
	"unsafe"

	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

func TestExtension(t *testing.T) {
	// We need to ensure that the the size of the data structures respects our
	// expectations, so we check this at test time. It's not worth it doing this at
	// runtime since the size of the data is not expected to change once compiled.
	var msg jsonProtoMessage
	require.Equal(t, int(unsafe.Sizeof(msg)), jsonProtoMessageSize,
		"Unexpected request struct size, this is a fatal error")

	require.Less(t, len(JSONProtoName), int(unsafe.Sizeof(msg.protocol_name)),
		"protocol name '%s' exceeds the maximum size", JSONProtoName)
}

//nolint:tparallel // Subtests can't run in parallel as they act on global data
func TestGdmExtensionSupport(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		advertisedExtensions []string
		checkExtensions      []string
		supportedExtensions  []string
	}{
		"Unknown extension is unsupported": {
			checkExtensions:     []string{"foo.extension"},
			supportedExtensions: nil,
		},
		"Extensions are advertised": {
			advertisedExtensions: []string{PamExtensionCustomJSON, "foo"},
			checkExtensions:      []string{PamExtensionCustomJSON, "foo"},
			supportedExtensions:  []string{PamExtensionCustomJSON, "foo"},
		},
		"The private string extension unsupported if not advertised": {
			checkExtensions:     []string{PamExtensionCustomJSON},
			supportedExtensions: nil,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// These tests can't be parallel since they act on env variables
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			AdvertisePamExtensions(tc.advertisedExtensions)
			t.Cleanup(func() { AdvertisePamExtensions(nil) })

			for _, ext := range tc.checkExtensions {
				shouldSupport := slices.Contains(tc.supportedExtensions, ext)
				require.Equal(t, shouldSupport, IsPamExtensionSupported(ext))
			}
		})
	}
}

func TestGdmJSONProto(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		value []byte
	}{
		"With null data": {
			value: []byte("null"),
		},
		"With single int": {
			value: []byte("55"),
		},
		"With single float": {
			value: []byte("5.5"),
		},
		"With single string": {
			value: []byte(`"hello"`),
		},
		"With single boolean": {
			value: []byte("true"),
		},
		"With empty object": {
			value: []byte("{}"),
		},
		"With complex object": {
			value: []byte(`{"type":"pollResponse","pollResponse":` +
				`[{"type":"brokerSelected","brokerSelected":{"brokerId":"a broker"}},` +
				`{"type":"authModeSelected","authModeSelected":{"authModeId":"auth mode"}}]}`),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			req, err := NewBinaryJSONProtoRequest(tc.value)
			require.NoError(t, err)
			t.Cleanup(req.Release)
			require.NotNil(t, req)
			require.NotNil(t, req.Pointer())
			require.Equal(t, pam.BinaryPrompt, req.Style())

			decoded, err := decodeJSONProtoMessage(req.Pointer())
			require.NoError(t, err)
			require.Equalf(t, tc.value, decoded, "JSON mismatch '%s' vs '%s'",
				string(tc.value), string(decoded))
		})
	}
}

func TestGdmJSONProtoRequestErrors(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		value []byte
	}{
		"With empty data": {
			value: []byte{},
		},
		"With null data": {
			value: nil,
		},
		"With single char": {
			value: []byte("m"),
		},
		"With lorem ipsum string data": {
			value: []byte(`
    Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod
	tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam,
	quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo
	consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse
	cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat
	non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.
`),
		},
		"With invalid JSON object": {
			value: []byte("{[,]}"),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			req, err := NewBinaryJSONProtoRequest(tc.value)
			require.Nil(t, req)
			require.ErrorIs(t, err, ErrInvalidJSON)
		})
	}
}

func TestGdmJSONProtoResponseErrors(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	testCases := map[string]struct {
		protoName    string
		protoVersion uint
		jsonValue    []byte

		wantError error
	}{
		"On proto name mismatch": {
			protoName:    "some.other.protocol",
			protoVersion: JSONProtoVersion,
			jsonValue:    []byte("null"),
			wantError:    ErrProtoNotSupported,
		},
		"On proto version mismatch": {
			protoName:    JSONProtoName,
			protoVersion: JSONProtoVersion + 100,
			jsonValue:    []byte("{}"),
			wantError:    ErrProtoNotSupported,
		},
		"On nil JSON": {
			protoName:    JSONProtoName,
			protoVersion: JSONProtoVersion,
			jsonValue:    nil,
			wantError:    ErrInvalidJSON,
		},
		"On empty JSON": {
			protoName:    JSONProtoName,
			protoVersion: JSONProtoVersion,
			jsonValue:    []byte{},
			wantError:    ErrInvalidJSON,
		},
		"On invalid JSON": {
			protoName:    JSONProtoName,
			protoVersion: JSONProtoVersion,
			jsonValue:    []byte("{]"),
			wantError:    ErrInvalidJSON,
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			req := allocateJSONProtoMessage()
			t.Cleanup(req.release)
			req.init(tc.protoName, tc.protoVersion, tc.jsonValue)
			require.Equal(t, req.protoVersion(), tc.protoVersion)
			require.Equal(t, req.protoName(), tc.protoName)

			binReq := pam.NewBinaryConvRequest(req.encode(), nil)
			t.Cleanup(binReq.Release)

			require.NotNil(t, binReq)
			require.NotNil(t, binReq.Pointer())
			require.Equal(t, pam.BinaryPrompt, binReq.Style())

			decoded, err := decodeJSONProtoMessage(binReq.Pointer())
			require.Nil(t, decoded)
			require.ErrorIs(t, err, tc.wantError)
		})
	}
}
