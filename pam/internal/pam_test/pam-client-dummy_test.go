package pam_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
)

var errTest = errors.New("an error")
var privateKey *rsa.PrivateKey

func TestAvailableBrokers(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client authd.PAMClient

		wantRet   *authd.ABResponse
		wantError error
	}{
		"With empty options": {
			client:  NewDummyClient(nil),
			wantRet: &authd.ABResponse{},
		},
		"With Error return value": {
			client:    NewDummyClient(nil, WithAvailableBrokers(nil, errTest)),
			wantError: errTest,
		},
		"With defined return value": {
			client: NewDummyClient(nil, WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
				{
					Id:        "testBroker",
					Name:      "A test broker",
					BrandIcon: ptrValue("/usr/share/icons/broker-icon.png"),
				},
			}, nil)),
			wantRet: &authd.ABResponse{
				BrokersInfos: []*authd.ABResponse_BrokerInfo{
					{
						Id:        "testBroker",
						Name:      "A test broker",
						BrandIcon: ptrValue("/usr/share/icons/broker-icon.png"),
					},
				},
			},
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ret, err := tc.client.AvailableBrokers(context.TODO(), nil)
			require.ErrorIs(t, err, tc.wantError)
			require.Equal(t, tc.wantRet, ret)
		})
	}
}

func TestGetPreviousBroker(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client authd.PAMClient
		args   *authd.GPBRequest

		wantRet   *authd.GPBResponse
		wantError error
	}{
		"With empty options": {
			client:  NewDummyClient(nil),
			wantRet: &authd.GPBResponse{},
		},
		"With Error return value": {
			client:    NewDummyClient(nil, WithGetPreviousBrokerReturn(nil, errTest)),
			wantError: errTest,
		},
		"With defined return value": {
			client: NewDummyClient(nil, WithGetPreviousBrokerReturn(ptrValue("my-previous-broker"), nil)),
			wantRet: &authd.GPBResponse{
				PreviousBroker: ptrValue("my-previous-broker"),
			},
		},
		"With defined empty return value": {
			client:  NewDummyClient(nil, WithGetPreviousBrokerReturn(nil, nil)),
			wantRet: &authd.GPBResponse{},
		},
		"With predefined default for user empty return value": {
			client: NewDummyClient(nil,
				WithPreviousBrokerForUser("user0", "broker0"),
				WithPreviousBrokerForUser("user1", "broker1"),
			),
			args:    &authd.GPBRequest{Username: "user1"},
			wantRet: &authd.GPBResponse{PreviousBroker: ptrValue("broker1")},
		},

		// Error cases
		"Error with missing user": {
			client:    NewDummyClient(nil, WithGetPreviousBrokerReturn(nil, nil)),
			args:      &authd.GPBRequest{},
			wantError: errors.New("no username provided"),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ret, err := tc.client.GetPreviousBroker(context.TODO(), tc.args)
			require.Equal(t, tc.wantError, err)
			require.Equal(t, tc.wantRet, ret)
		})
	}
}

func TestSelectBroker(t *testing.T) {
	t.Parallel()

	pubASN1, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	wantEncryptionKey := base64.StdEncoding.EncodeToString(pubASN1)

	testCases := map[string]struct {
		client            authd.PAMClient
		args              *authd.SBRequest
		reselectAgainUser string

		wantGeneratedSessionID bool
		wantRet                *authd.SBResponse
		wantError              error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithSelectBrokerReturn(nil, errTest)),
			wantError: errTest,
		},
		"With valid args and generated return value": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(nil, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args:                   &authd.SBRequest{BrokerId: "test-broker"},
			wantGeneratedSessionID: true,
		},
		"With valid args and empty return value": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(&authd.SBResponse{}, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args:                   &authd.SBRequest{BrokerId: "test-broker"},
			wantRet:                &authd.SBResponse{},
			wantGeneratedSessionID: true,
		},
		"With valid args and empty return value with ignored ID generation": {
			client: NewDummyClient(nil,
				WithIgnoreSessionIDGeneration(),
				WithSelectBrokerReturn(&authd.SBResponse{}, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args:    &authd.SBRequest{BrokerId: "test-broker"},
			wantRet: &authd.SBResponse{},
		},
		"With valid args and defined return value": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(&authd.SBResponse{
					SessionId:     "session-id",
					EncryptionKey: "super-secret-encryption-key",
				}, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &authd.SBRequest{BrokerId: "test-broker"},
			wantRet: &authd.SBResponse{
				SessionId:     "session-id",
				EncryptionKey: "super-secret-encryption-key",
			},
		},
		"With private key and valid args and empty return value": {
			client: NewDummyClient(privateKey,
				WithSelectBrokerReturn(&authd.SBResponse{}, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &authd.SBRequest{BrokerId: "test-broker"},
			wantRet: &authd.SBResponse{
				EncryptionKey: wantEncryptionKey,
			},
			wantGeneratedSessionID: true,
		},
		"With private key and valid args, empty return value ignoring session ID generation": {
			client: NewDummyClient(privateKey,
				WithIgnoreSessionIDGeneration(),
				WithSelectBrokerReturn(&authd.SBResponse{}, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &authd.SBRequest{BrokerId: "test-broker"},
			wantRet: &authd.SBResponse{
				EncryptionKey: wantEncryptionKey,
			},
		},
		"With private key and valid args and defined return value": {
			client: NewDummyClient(privateKey,
				WithSelectBrokerReturn(&authd.SBResponse{
					SessionId:     "session-id",
					EncryptionKey: "super-secret-encryption-key",
				}, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "dummy-broker",
						Name: "A Dummy broker",
					},
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &authd.SBRequest{BrokerId: "test-broker"},
			wantRet: &authd.SBResponse{
				SessionId:     "session-id",
				EncryptionKey: "super-secret-encryption-key",
			},
		},
		"With private key and valid args and defined return value without encryption key": {
			client: NewDummyClient(privateKey,
				WithSelectBrokerReturn(&authd.SBResponse{
					SessionId: "session-id",
				}, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "dummy-broker",
						Name: "A Dummy broker",
					},
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &authd.SBRequest{BrokerId: "test-broker"},
			wantRet: &authd.SBResponse{
				SessionId:     "session-id",
				EncryptionKey: wantEncryptionKey,
			},
		},
		"Starting a session for same user is fine": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(nil, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &authd.SBRequest{
				BrokerId: "test-broker",
				Username: "an-user",
			},
			reselectAgainUser:      "an-user",
			wantGeneratedSessionID: true,
			wantRet:                &authd.SBResponse{},
		},
		"Starting a session for another user is fine when ignoring ID checks": {
			client: NewDummyClient(nil,
				WithIgnoreSessionIDChecks(),
				WithIgnoreSessionIDGeneration(),
				WithSelectBrokerReturn(nil, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &authd.SBRequest{
				BrokerId: "test-broker",
				Username: "an-user",
			},
			reselectAgainUser: "another-user",
			wantRet:           &authd.SBResponse{},
		},

		// Error cases
		"Error with nil args and empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"Error with empty args empty options": {
			client:    NewDummyClient(nil),
			args:      &authd.SBRequest{},
			wantError: errors.New("no broker ID provided"),
		},
		"Error on unknown broker id": {
			client:    NewDummyClient(nil, WithSelectBrokerReturn(nil, nil)),
			args:      &authd.SBRequest{BrokerId: "some-broker"},
			wantError: fmt.Errorf(`broker "some-broker" not found`),
		},
		"Error on starting a session again": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(nil, nil),
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &authd.SBRequest{
				BrokerId: "test-broker",
				Username: "an-user",
			},
			reselectAgainUser:      "another-user",
			wantGeneratedSessionID: true,
		},
		"Error on broker fetching failed": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(nil, nil),
				WithAvailableBrokers(nil, errTest)),
			args:      &authd.SBRequest{BrokerId: "test-broker"},
			wantError: errTest,
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ret, err := tc.client.SelectBroker(context.TODO(), tc.args)
			require.Equal(t, tc.wantError, err)
			if tc.wantGeneratedSessionID {
				require.NotNil(t, ret)
				_, err := uuid.Parse(ret.SessionId)
				require.NoError(t, err)
				if tc.wantRet != nil {
					tc.wantRet.SessionId = ret.SessionId
				} else {
					tc.wantRet = ret
				}
			}
			require.Equal(t, tc.wantRet, ret)
			if err != nil {
				require.Nil(t, ret)
				return
			}

			dc, ok := tc.client.(*DummyClient)
			require.True(t, ok, "Provided client is not a Dummy client")
			if ret != nil {
				require.Equal(t, ret.SessionId, dc.CurrentSessionID())
			}

			if tc.args == nil {
				require.Empty(t, dc.SelectedBrokerID())
				require.Empty(t, dc.SelectedLang())
				require.Empty(t, dc.SelectedUsername())
				return
			}
			require.Equal(t, tc.args.GetBrokerId(), dc.SelectedBrokerID())
			require.Equal(t, tc.args.GetLang(), dc.SelectedLang())
			require.Equal(t, tc.args.GetUsername(), dc.SelectedUsername())

			if tc.reselectAgainUser != "" {
				ret, err := tc.client.SelectBroker(context.TODO(), tc.args)
				require.Nil(t, err)
				require.Equal(t, tc.wantRet, ret)
			}
		})
	}
}

func startBrokerSession(t *testing.T, client authd.PAMClient, brokerIdx int) string {
	t.Helper()

	brokers, err := client.AvailableBrokers(context.TODO(), nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(brokers.BrokersInfos), brokerIdx)
	ret, err := client.SelectBroker(context.TODO(), &authd.SBRequest{
		BrokerId: brokers.BrokersInfos[brokerIdx].Id,
	})
	require.NoError(t, err)
	require.NotNil(t, ret)
	return ret.SessionId
}

func TestGetAuthenticationModes(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client              authd.PAMClient
		args                *authd.GAMRequest
		skipBrokerSelection bool

		wantRet   *authd.GAMResponse
		wantError error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithGetAuthenticationModesReturn(nil, errTest)),
			wantError: errTest,
		},
		"With empty return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithGetAuthenticationModesReturn(nil, nil),
			),
			args:    &authd.GAMRequest{SessionId: "started-session-id"},
			wantRet: &authd.GAMResponse{AuthenticationModes: []*authd.GAMResponse_AuthenticationMode{}},
		},
		"With all modes return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithGetAuthenticationModesReturn([]*authd.GAMResponse_AuthenticationMode{
					{
						Id:    "foo",
						Label: "Bar",
					},
					{
						Id:    "password",
						Label: "Insert your password",
					},
				}, nil),
			),
			args: &authd.GAMRequest{SessionId: "started-session-id"},
			wantRet: &authd.GAMResponse{
				AuthenticationModes: []*authd.GAMResponse_AuthenticationMode{
					{
						Id:    "foo",
						Label: "Bar",
					},
					{
						Id:    "password",
						Label: "Insert your password",
					},
				},
			},
		},
		"With modes returned from values": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithUILayout("foobar", "Baz", &authd.UILayout{}),
				WithUILayout("password", "Insert your password", FormUILayout()),
			),
			args: &authd.GAMRequest{SessionId: "started-session-id"},
			wantRet: &authd.GAMResponse{
				AuthenticationModes: []*authd.GAMResponse_AuthenticationMode{
					{
						Id:    "foobar",
						Label: "Baz",
					},
					{
						Id:    "password",
						Label: "Insert your password",
					},
				},
			},
		},
		"With no session ID arg when enabled": {
			client:  NewDummyClient(nil, WithIgnoreSessionIDChecks()),
			args:    &authd.GAMRequest{},
			wantRet: &authd.GAMResponse{AuthenticationModes: []*authd.GAMResponse_AuthenticationMode{}},
		},

		// Error cases
		"Error with nil args and empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"Error with no session ID arg": {
			client:    NewDummyClient(nil),
			args:      &authd.GAMRequest{},
			wantError: errors.New("no session ID provided"),
		},
		"Error with not-matching session ID": {
			client:              NewDummyClient(nil),
			args:                &authd.GAMRequest{SessionId: "session-id"},
			skipBrokerSelection: true,
			wantError:           errors.New(`impossible to get authentication mode, session ID "session-id" not found`),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.args != nil && tc.args.SessionId != "" && !tc.skipBrokerSelection {
				sessionID := startBrokerSession(t, tc.client, 0)
				require.Equal(t, tc.args.SessionId, sessionID)
			}

			ret, err := tc.client.GetAuthenticationModes(context.TODO(), tc.args)
			require.Equal(t, tc.wantError, err)
			if err != nil {
				require.Nil(t, ret)
				return
			}

			require.Equal(t, tc.wantRet, ret)
		})
	}
}

func TestSelectAuthenticationModes(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client              authd.PAMClient
		args                *authd.SAMRequest
		skipBrokerSelection bool

		wantRet   *authd.SAMResponse
		wantError error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithSelectAuthenticationModeReturn(nil, errTest)),
			wantError: errTest,
		},
		"With empty return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithSelectAuthenticationModeReturn(&authd.UILayout{}, nil),
			),
			args:    &authd.SAMRequest{SessionId: "started-session-id"},
			wantRet: &authd.SAMResponse{UiLayoutInfo: &authd.UILayout{}},
		},
		"With all modes return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithUILayout("password", "Write the password", FormUILayout()),
				WithUILayout("pin", "Write the PIN number", FormUILayout()),
				WithUILayout("qrcode", "Scan the QR code", QrCodeUILayout()),
				WithUILayout("new-pass", "Update your password", NewPasswordUILayout()),
			),
			args: &authd.SAMRequest{
				SessionId:            "started-session-id",
				AuthenticationModeId: "qrcode",
			},
			wantRet: &authd.SAMResponse{UiLayoutInfo: QrCodeUILayout()},
		},

		// Error cases
		"Error with nil args and empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"Error with no session ID arg": {
			client:    NewDummyClient(nil),
			args:      &authd.SAMRequest{},
			wantError: errors.New("no session ID provided"),
		},
		"Error with not-matching session ID": {
			client:              NewDummyClient(nil),
			args:                &authd.SAMRequest{SessionId: "session-id"},
			skipBrokerSelection: true,
			wantError:           errors.New(`impossible to select authentication mode, session ID "session-id" not found`),
		},
		"Error with no authentication mode ID": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithSelectAuthenticationModeReturn(nil, nil),
			),
			args:      &authd.SAMRequest{SessionId: "started-session-id"},
			wantError: errors.New("no authentication mode ID provided"),
		},
		"Error unknown authentication mode ID": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithSelectAuthenticationModeReturn(nil, nil),
			),
			args: &authd.SAMRequest{
				SessionId:            "started-session-id",
				AuthenticationModeId: "auth-mode-id",
			},
			wantError: errors.New(`authentication mode "auth-mode-id" not found`),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.args != nil && tc.args.SessionId != "" && !tc.skipBrokerSelection {
				sessionID := startBrokerSession(t, tc.client, 0)
				require.Equal(t, tc.args.SessionId, sessionID)
			}

			ret, err := tc.client.SelectAuthenticationMode(context.TODO(), tc.args)
			require.Equal(t, tc.wantError, err)
			require.Equal(t, tc.wantRet, ret)
		})
	}
}

func TestIsAuthenticated(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client              authd.PAMClient
		args                *authd.IARequest
		skipBrokerSelection bool

		wantRet   *authd.IAResponse
		wantError error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithIsAuthenticatedReturn(nil, errTest)),
			wantError: errTest,
		},
		"With empty return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedReturn(&authd.IAResponse{}, nil),
			),
			args:    &authd.IARequest{SessionId: "started-session-id"},
			wantRet: &authd.IAResponse{},
		},
		"With retry return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedReturn(&authd.IAResponse{
					Access: brokers.AuthRetry,
					Msg:    "Try again",
				}, nil),
			),
			args: &authd.IARequest{SessionId: "started-session-id"},
			wantRet: &authd.IAResponse{
				Access: brokers.AuthRetry,
				Msg:    "Try again",
			},
		},
		"Invalid challenge": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "invalid-password"),
					},
				},
			},
			wantRet: &authd.IAResponse{
				Access: brokers.AuthDenied,
			},
		},
		"Invalid challenge with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
				WithIsAuthenticatedMessage("You're out!"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "invalid-password"),
					},
				},
			},
			wantRet: &authd.IAResponse{
				Access: brokers.AuthDenied,
				Msg:    `{"message": "You're out!"}`,
			},
		},
		"Retry challenge with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
				WithIsAuthenticatedMaxRetries(1),
				WithIsAuthenticatedMessage("try again!"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "invalid-password"),
					},
				},
			},
			wantRet: &authd.IAResponse{
				Access: brokers.AuthRetry,
				Msg:    `{"message": "try again!"}`,
			},
		},
		"Valid challenge": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "super-secret-password"),
					},
				},
			},
			wantRet: &authd.IAResponse{
				Access: brokers.AuthGranted,
			},
		},
		"Valid challenge with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
				WithIsAuthenticatedMessage("try again!"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "super-secret-password"),
					},
				},
			},
			wantRet: &authd.IAResponse{
				Access: brokers.AuthGranted,
				Msg:    `{"message": "try again!"}`,
			},
		},
		"Wait with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantWait(time.Microsecond*5),
				WithIsAuthenticatedMessage("Wait done!"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Wait{Wait: "true"},
				},
			},
			wantRet: &authd.IAResponse{
				Access: brokers.AuthGranted,
				Msg:    `{"message": "Wait done!"}`,
			},
		},
		"Skip with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantSkip(),
				WithIsAuthenticatedMessage("Skip done!"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Skip{Skip: "true"},
				},
			},
			wantRet: &authd.IAResponse{
				Msg: `{"message": "Skip done!"}`,
			},
		},

		// Error cases
		"Error with nil args and empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"Error with no session ID arg": {
			client:    NewDummyClient(nil),
			args:      &authd.IARequest{},
			wantError: errors.New("no session ID provided"),
		},
		"Error with not-matching session ID": {
			client:              NewDummyClient(nil),
			args:                &authd.IARequest{SessionId: "session-id"},
			skipBrokerSelection: true,
			wantError:           errors.New(`impossible to authenticate, session ID "session-id" not found`),
		},
		"Error with no authentication data": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
			),
			args:      &authd.IARequest{SessionId: "started-session-id"},
			wantError: errors.New("no authentication data provided"),
		},
		"Error with invalid authentication data": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
			),
			args: &authd.IARequest{
				SessionId:          "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{},
			},
			wantError: errors.New("no authentication data provided"),
		},
		"Error missing wanted challenge": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{},
				},
			},
			wantError: errors.New("no wanted challenge provided"),
		},
		"Error missing wanted wait": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Wait{},
				},
			},
			wantError: errors.New("no wanted wait provided"),
		},
		"Error missing wanted skip": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Skip{},
				},
			},
			wantError: errors.New("no wanted skip requested"),
		},
		"Error empty challenge": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("challenge"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{},
				},
			},
			wantError: errors.New("no challenge provided"),
		},
		"Error decoding challenge": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("challenge"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{
						Challenge: "Invalid base64",
					},
				},
			},
			wantError: base64.CorruptInputError(7),
		},
		"Error decrypting challenge per missing private key": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("challenge"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{
						Challenge: base64.StdEncoding.EncodeToString([]byte("Invalid encrypted key")),
					},
				},
			},
			wantError: errors.New("no private key defined"),
		},
		"Error decrypting invalid challenge": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("challenge"),
			),
			args: &authd.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &authd.IARequest_AuthenticationData{
					Item: &authd.IARequest_AuthenticationData_Challenge{
						Challenge: base64.StdEncoding.EncodeToString([]byte("Invalid encrypted key")),
					},
				},
			},
			wantError: rsa.ErrDecryption,
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.args != nil && tc.args.SessionId != "" && !tc.skipBrokerSelection {
				sessionID := startBrokerSession(t, tc.client, 0)
				require.Equal(t, tc.args.SessionId, sessionID)
			}

			ret, err := tc.client.IsAuthenticated(context.TODO(), tc.args)
			require.Equal(t, tc.wantError, err)
			require.Equal(t, tc.wantRet, ret)
		})
	}
}

func encryptAndEncodeChallenge(t *testing.T, pubKey *rsa.PublicKey, challenge string) string {
	t.Helper()

	ciphertext, err := rsa.EncryptOAEP(sha512.New(), rand.Reader, pubKey, []byte(challenge), nil)
	require.NoError(t, err)

	// encrypt it to base64 and replace the challenge with it
	base64Encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return base64Encoded
}

func TestEndSession(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client           authd.PAMClient
		args             *authd.ESRequest
		selectedBrokerID string

		wantError error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithEndSessionReturn(errTest)),
			wantError: errTest,
		},
		"With valid return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{{
					Id:        "test-broker",
					Name:      "A test broker",
					BrandIcon: ptrValue("/usr/share/icons/broker-icon.png"),
				}}, nil),
				WithSelectBrokerReturn(&authd.SBResponse{SessionId: "started-session-id"}, nil),
				WithEndSessionReturn(nil),
			),
			args:             &authd.ESRequest{SessionId: "started-session-id"},
			selectedBrokerID: "test-broker",
		},

		// Error cases
		"Error with nil args and empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"Error with empty args empty options": {
			client:    NewDummyClient(nil),
			args:      &authd.ESRequest{},
			wantError: errors.New("no session ID provided"),
		},
		"Error with not-matching session ID": {
			client:    NewDummyClient(nil),
			args:      &authd.ESRequest{SessionId: "a-session-id"},
			wantError: errors.New(`impossible to end session "a-session-id", not found`),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dc, ok := tc.client.(*DummyClient)
			require.True(t, ok, "Provided client is not a Dummy client")

			if tc.args != nil && tc.args.SessionId != "" && tc.selectedBrokerID != "" {
				ret, err := tc.client.SelectBroker(context.TODO(), &authd.SBRequest{
					BrokerId: tc.selectedBrokerID,
				})
				require.NoError(t, err)
				require.NotNil(t, ret)
				require.Equal(t, tc.args.SessionId, ret.SessionId)
				require.Equal(t, tc.args.SessionId, dc.CurrentSessionID())
			}

			ret, err := tc.client.EndSession(context.TODO(), tc.args)
			require.Equal(t, tc.wantError, err)
			if err != nil {
				require.Nil(t, ret)
				return
			}

			require.Equal(t, &authd.Empty{}, ret)
			require.Empty(t, dc.CurrentSessionID())
			require.Empty(t, dc.SelectedUsername())
		})
	}
}

func TestSetDefaultBrokerForUser(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client authd.PAMClient
		args   *authd.SDBFURequest

		wantError error
	}{
		"With empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"With Error return value": {
			client:    NewDummyClient(nil, WithSetDefaultBrokerReturn(errTest)),
			wantError: errTest,
		},
		"With valid arguments": {
			client: NewDummyClient(nil, WithSetDefaultBrokerReturn(nil)),
			args: &authd.SDBFURequest{
				BrokerId: "broker-id",
				Username: "username",
			},
		},

		// Error cases
		"Error if no user name is provided": {
			client:    NewDummyClient(nil),
			args:      &authd.SDBFURequest{BrokerId: "broker-id"},
			wantError: errors.New("no valid username provided"),
		},
		"Error if no broker ID is provided": {
			client:    NewDummyClient(nil),
			args:      &authd.SDBFURequest{Username: "username"},
			wantError: errors.New("no valid broker ID provided"),
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ret, err := tc.client.SetDefaultBrokerForUser(context.TODO(), tc.args)
			require.Equal(t, err, tc.wantError)
			if err != nil {
				require.Nil(t, ret)
				return
			}

			require.Equal(t, &authd.Empty{}, ret)

			if tc.args == nil {
				return
			}
			retBroker, err := tc.client.GetPreviousBroker(context.TODO(),
				&authd.GPBRequest{Username: tc.args.Username})
			require.NoError(t, err)
			require.Equal(t, tc.args.BrokerId, *retBroker.PreviousBroker)
		})
	}
}

func TestMain(m *testing.M) {
	var err error
	privateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("could not create an valid rsa key: %v", err))
	}
	os.Exit(m.Run())
}
