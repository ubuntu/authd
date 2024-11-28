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
	"github.com/ubuntu/authd/brokers/auth"
	"github.com/ubuntu/authd/brokers/layouts"
	"github.com/ubuntu/authd/internal/proto"
)

var errTest = errors.New("an error")
var privateKey *rsa.PrivateKey

func TestAvailableBrokers(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client proto.PAMClient

		wantRet   *proto.ABResponse
		wantError error
	}{
		"With empty options": {
			client:  NewDummyClient(nil),
			wantRet: &proto.ABResponse{},
		},
		"With Error return value": {
			client:    NewDummyClient(nil, WithAvailableBrokers(nil, errTest)),
			wantError: errTest,
		},
		"With defined return value": {
			client: NewDummyClient(nil, WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
				{
					Id:        "testBroker",
					Name:      "A test broker",
					BrandIcon: ptrValue("/usr/share/icons/broker-icon.png"),
				},
			}, nil)),
			wantRet: &proto.ABResponse{
				BrokersInfos: []*proto.ABResponse_BrokerInfo{
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
		client proto.PAMClient
		args   *proto.GPBRequest

		wantRet   *proto.GPBResponse
		wantError error
	}{
		"With empty options": {
			client:  NewDummyClient(nil),
			wantRet: &proto.GPBResponse{},
		},
		"With Error return value": {
			client:    NewDummyClient(nil, WithGetPreviousBrokerReturn("", errTest)),
			wantError: errTest,
		},
		"With defined return value": {
			client: NewDummyClient(nil, WithGetPreviousBrokerReturn("my-previous-broker", nil)),
			wantRet: &proto.GPBResponse{
				PreviousBroker: "my-previous-broker",
			},
		},
		"With defined empty return value": {
			client:  NewDummyClient(nil, WithGetPreviousBrokerReturn("", nil)),
			wantRet: &proto.GPBResponse{},
		},
		"With predefined default for user empty return value": {
			client: NewDummyClient(nil,
				WithPreviousBrokerForUser("user0", "broker0"),
				WithPreviousBrokerForUser("user1", "broker1"),
			),
			args:    &proto.GPBRequest{Username: "user1"},
			wantRet: &proto.GPBResponse{PreviousBroker: "broker1"},
		},

		// Error cases
		"Error with missing user": {
			client:    NewDummyClient(nil, WithGetPreviousBrokerReturn("", nil)),
			args:      &proto.GPBRequest{},
			wantError: errors.New("no username provided"),
		},
	}
	for name, tc := range testCases {
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
		client            proto.PAMClient
		args              *proto.SBRequest
		reselectAgainUser string

		wantGeneratedSessionID bool
		wantRet                *proto.SBResponse
		wantError              error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithSelectBrokerReturn(nil, errTest)),
			wantError: errTest,
		},
		"With valid args and generated return value": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(nil, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args:                   &proto.SBRequest{BrokerId: "test-broker"},
			wantGeneratedSessionID: true,
		},
		"With valid args and empty return value": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(&proto.SBResponse{}, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args:                   &proto.SBRequest{BrokerId: "test-broker"},
			wantRet:                &proto.SBResponse{},
			wantGeneratedSessionID: true,
		},
		"With valid args and empty return value with ignored ID generation": {
			client: NewDummyClient(nil,
				WithIgnoreSessionIDGeneration(),
				WithSelectBrokerReturn(&proto.SBResponse{}, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args:    &proto.SBRequest{BrokerId: "test-broker"},
			wantRet: &proto.SBResponse{},
		},
		"With valid args and defined return value": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(&proto.SBResponse{
					SessionId:     "session-id",
					EncryptionKey: "super-secret-encryption-key",
				}, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &proto.SBRequest{BrokerId: "test-broker"},
			wantRet: &proto.SBResponse{
				SessionId:     "session-id",
				EncryptionKey: "super-secret-encryption-key",
			},
		},
		"With private key and valid args and empty return value": {
			client: NewDummyClient(privateKey,
				WithSelectBrokerReturn(&proto.SBResponse{}, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &proto.SBRequest{BrokerId: "test-broker"},
			wantRet: &proto.SBResponse{
				EncryptionKey: wantEncryptionKey,
			},
			wantGeneratedSessionID: true,
		},
		"With private key and valid args, empty return value ignoring session ID generation": {
			client: NewDummyClient(privateKey,
				WithIgnoreSessionIDGeneration(),
				WithSelectBrokerReturn(&proto.SBResponse{}, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &proto.SBRequest{BrokerId: "test-broker"},
			wantRet: &proto.SBResponse{
				EncryptionKey: wantEncryptionKey,
			},
		},
		"With private key and valid args and defined return value": {
			client: NewDummyClient(privateKey,
				WithSelectBrokerReturn(&proto.SBResponse{
					SessionId:     "session-id",
					EncryptionKey: "super-secret-encryption-key",
				}, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "dummy-broker",
						Name: "A Dummy broker",
					},
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &proto.SBRequest{BrokerId: "test-broker"},
			wantRet: &proto.SBResponse{
				SessionId:     "session-id",
				EncryptionKey: "super-secret-encryption-key",
			},
		},
		"With private key and valid args and defined return value without encryption key": {
			client: NewDummyClient(privateKey,
				WithSelectBrokerReturn(&proto.SBResponse{
					SessionId: "session-id",
				}, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "dummy-broker",
						Name: "A Dummy broker",
					},
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &proto.SBRequest{BrokerId: "test-broker"},
			wantRet: &proto.SBResponse{
				SessionId:     "session-id",
				EncryptionKey: wantEncryptionKey,
			},
		},
		"Starting a session for same user is fine": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(nil, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &proto.SBRequest{
				BrokerId: "test-broker",
				Username: "an-user",
			},
			reselectAgainUser:      "an-user",
			wantGeneratedSessionID: true,
			wantRet:                &proto.SBResponse{},
		},
		"Starting a session for another user is fine when ignoring ID checks": {
			client: NewDummyClient(nil,
				WithIgnoreSessionIDChecks(),
				WithIgnoreSessionIDGeneration(),
				WithSelectBrokerReturn(nil, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &proto.SBRequest{
				BrokerId: "test-broker",
				Username: "an-user",
			},
			reselectAgainUser: "another-user",
			wantRet:           &proto.SBResponse{},
		},

		// Error cases
		"Error with nil args and empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"Error with empty args empty options": {
			client:    NewDummyClient(nil),
			args:      &proto.SBRequest{},
			wantError: errors.New("no broker ID provided"),
		},
		"Error on unknown broker id": {
			client:    NewDummyClient(nil, WithSelectBrokerReturn(nil, nil)),
			args:      &proto.SBRequest{BrokerId: "some-broker"},
			wantError: fmt.Errorf(`broker "some-broker" not found`),
		},
		"Error on starting a session again": {
			client: NewDummyClient(nil,
				WithSelectBrokerReturn(nil, nil),
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{
					{
						Id:   "test-broker",
						Name: "A test broker",
					},
				}, nil)),
			args: &proto.SBRequest{
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
			args:      &proto.SBRequest{BrokerId: "test-broker"},
			wantError: errTest,
		},
	}
	for name, tc := range testCases {
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

func startBrokerSession(t *testing.T, client proto.PAMClient, brokerIdx int) string {
	t.Helper()

	brokers, err := client.AvailableBrokers(context.TODO(), nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(brokers.BrokersInfos), brokerIdx)
	ret, err := client.SelectBroker(context.TODO(), &proto.SBRequest{
		BrokerId: brokers.BrokersInfos[brokerIdx].Id,
	})
	require.NoError(t, err)
	require.NotNil(t, ret)
	return ret.SessionId
}

func TestGetAuthenticationModes(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client              proto.PAMClient
		args                *proto.GAMRequest
		skipBrokerSelection bool

		wantRet   *proto.GAMResponse
		wantError error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithGetAuthenticationModesReturn(nil, errTest)),
			wantError: errTest,
		},
		"With empty return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithGetAuthenticationModesReturn(nil, nil),
			),
			args:    &proto.GAMRequest{SessionId: "started-session-id"},
			wantRet: &proto.GAMResponse{AuthenticationModes: []*proto.GAMResponse_AuthenticationMode{}},
		},
		"With all modes return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithGetAuthenticationModesReturn([]*proto.GAMResponse_AuthenticationMode{
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
			args: &proto.GAMRequest{SessionId: "started-session-id"},
			wantRet: &proto.GAMResponse{
				AuthenticationModes: []*proto.GAMResponse_AuthenticationMode{
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
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithUILayout("foobar", "Baz", &proto.UILayout{}),
				WithUILayout("password", "Insert your password", FormUILayout()),
			),
			args: &proto.GAMRequest{SessionId: "started-session-id"},
			wantRet: &proto.GAMResponse{
				AuthenticationModes: []*proto.GAMResponse_AuthenticationMode{
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
			args:    &proto.GAMRequest{},
			wantRet: &proto.GAMResponse{AuthenticationModes: []*proto.GAMResponse_AuthenticationMode{}},
		},

		// Error cases
		"Error with nil args and empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"Error with no session ID arg": {
			client:    NewDummyClient(nil),
			args:      &proto.GAMRequest{},
			wantError: errors.New("no session ID provided"),
		},
		"Error with not-matching session ID": {
			client:              NewDummyClient(nil),
			args:                &proto.GAMRequest{SessionId: "session-id"},
			skipBrokerSelection: true,
			wantError:           errors.New(`impossible to get authentication mode, session ID "session-id" not found`),
		},
	}
	for name, tc := range testCases {
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
		client              proto.PAMClient
		args                *proto.SAMRequest
		skipBrokerSelection bool

		wantRet   *proto.SAMResponse
		wantError error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithSelectAuthenticationModeReturn(nil, errTest)),
			wantError: errTest,
		},
		"With empty return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithSelectAuthenticationModeReturn(&proto.UILayout{}, nil),
			),
			args:    &proto.SAMRequest{SessionId: "started-session-id"},
			wantRet: &proto.SAMResponse{UiLayoutInfo: &proto.UILayout{}},
		},
		"With all modes return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithUILayout("password", "Write the password", FormUILayout()),
				WithUILayout("pin", "Write the PIN number", FormUILayout()),
				WithUILayout(layouts.QrCode, "Scan the QR code", QrCodeUILayout()),
				WithUILayout("new-pass", "Update your password", NewPasswordUILayout()),
			),
			args: &proto.SAMRequest{
				SessionId:            "started-session-id",
				AuthenticationModeId: layouts.QrCode,
			},
			wantRet: &proto.SAMResponse{UiLayoutInfo: QrCodeUILayout()},
		},

		// Error cases
		"Error with nil args and empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"Error with no session ID arg": {
			client:    NewDummyClient(nil),
			args:      &proto.SAMRequest{},
			wantError: errors.New("no session ID provided"),
		},
		"Error with not-matching session ID": {
			client:              NewDummyClient(nil),
			args:                &proto.SAMRequest{SessionId: "session-id"},
			skipBrokerSelection: true,
			wantError:           errors.New(`impossible to select authentication mode, session ID "session-id" not found`),
		},
		"Error with no authentication mode ID": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithSelectAuthenticationModeReturn(nil, nil),
			),
			args:      &proto.SAMRequest{SessionId: "started-session-id"},
			wantError: errors.New("no authentication mode ID provided"),
		},
		"Error unknown authentication mode ID": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithSelectAuthenticationModeReturn(nil, nil),
			),
			args: &proto.SAMRequest{
				SessionId:            "started-session-id",
				AuthenticationModeId: "auth-mode-id",
			},
			wantError: errors.New(`authentication mode "auth-mode-id" not found`),
		},
	}
	for name, tc := range testCases {
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
		client              proto.PAMClient
		args                *proto.IARequest
		skipBrokerSelection bool

		wantRet   *proto.IAResponse
		wantError error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithIsAuthenticatedReturn(nil, errTest)),
			wantError: errTest,
		},
		"With empty return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedReturn(&proto.IAResponse{}, nil),
			),
			args:    &proto.IARequest{SessionId: "started-session-id"},
			wantRet: &proto.IAResponse{},
		},
		"With retry return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedReturn(&proto.IAResponse{
					Access: auth.Retry,
					Msg:    "Try again",
				}, nil),
			),
			args: &proto.IARequest{SessionId: "started-session-id"},
			wantRet: &proto.IAResponse{
				Access: auth.Retry,
				Msg:    "Try again",
			},
		},
		"Invalid challenge": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "invalid-password"),
					},
				},
			},
			wantRet: &proto.IAResponse{
				Access: auth.Denied,
			},
		},
		"Invalid challenge with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
				WithIsAuthenticatedMessage("You're out!"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "invalid-password"),
					},
				},
			},
			wantRet: &proto.IAResponse{
				Access: auth.Denied,
				Msg:    `{"message": "You're out!"}`,
			},
		},
		"Retry challenge with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
				WithIsAuthenticatedMaxRetries(1),
				WithIsAuthenticatedMessage("try again!"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "invalid-password"),
					},
				},
			},
			wantRet: &proto.IAResponse{
				Access: auth.Retry,
				Msg:    `{"message": "try again!"}`,
			},
		},
		"Valid challenge": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "super-secret-password"),
					},
				},
			},
			wantRet: &proto.IAResponse{
				Access: auth.Granted,
			},
		},
		"Valid challenge with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
				WithIsAuthenticatedMessage("try again!"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{
						Challenge: encryptAndEncodeChallenge(t, &privateKey.PublicKey, "super-secret-password"),
					},
				},
			},
			wantRet: &proto.IAResponse{
				Access: auth.Granted,
				Msg:    `{"message": "try again!"}`,
			},
		},
		"Wait with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantWait(time.Microsecond*5),
				WithIsAuthenticatedMessage("Wait done!"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Wait{Wait: layouts.True},
				},
			},
			wantRet: &proto.IAResponse{
				Access: auth.Granted,
				Msg:    `{"message": "Wait done!"}`,
			},
		},
		"Skip with message": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantSkip(),
				WithIsAuthenticatedMessage("Skip done!"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Skip{Skip: layouts.True},
				},
			},
			wantRet: &proto.IAResponse{
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
			args:      &proto.IARequest{},
			wantError: errors.New("no session ID provided"),
		},
		"Error with not-matching session ID": {
			client:              NewDummyClient(nil),
			args:                &proto.IARequest{SessionId: "session-id"},
			skipBrokerSelection: true,
			wantError:           errors.New(`impossible to authenticate, session ID "session-id" not found`),
		},
		"Error with no authentication data": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
			),
			args:      &proto.IARequest{SessionId: "started-session-id"},
			wantError: errors.New("no authentication data provided"),
		},
		"Error with invalid authentication data": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("super-secret-password"),
			),
			args: &proto.IARequest{
				SessionId:          "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{},
			},
			wantError: errors.New("no authentication data provided"),
		},
		"Error missing wanted challenge": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{},
				},
			},
			wantError: errors.New("no wanted challenge provided"),
		},
		"Error missing wanted wait": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Wait{},
				},
			},
			wantError: errors.New("no wanted wait provided"),
		},
		"Error missing wanted skip": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Skip{},
				},
			},
			wantError: errors.New("no wanted skip requested"),
		},
		"Error empty challenge": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("challenge"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{},
				},
			},
			wantError: errors.New("no challenge provided"),
		},
		"Error decoding challenge": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("challenge"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{
						Challenge: "Invalid base64",
					},
				},
			},
			wantError: base64.CorruptInputError(7),
		},
		"Error decrypting challenge per missing private key": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("challenge"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{
						Challenge: base64.StdEncoding.EncodeToString([]byte("Invalid encrypted key")),
					},
				},
			},
			wantError: errors.New("no private key defined"),
		},
		"Error decrypting invalid challenge": {
			client: NewDummyClient(privateKey,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:   "test-broker",
					Name: "A test broker",
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithIsAuthenticatedWantChallenge("challenge"),
			),
			args: &proto.IARequest{
				SessionId: "started-session-id",
				AuthenticationData: &proto.IARequest_AuthenticationData{
					Item: &proto.IARequest_AuthenticationData_Challenge{
						Challenge: base64.StdEncoding.EncodeToString([]byte("Invalid encrypted key")),
					},
				},
			},
			wantError: rsa.ErrDecryption,
		},
	}
	for name, tc := range testCases {
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
		client           proto.PAMClient
		args             *proto.ESRequest
		selectedBrokerID string

		wantError error
	}{
		"With Error return value": {
			client:    NewDummyClient(nil, WithEndSessionReturn(errTest)),
			wantError: errTest,
		},
		"With valid return value": {
			client: NewDummyClient(nil,
				WithAvailableBrokers([]*proto.ABResponse_BrokerInfo{{
					Id:        "test-broker",
					Name:      "A test broker",
					BrandIcon: ptrValue("/usr/share/icons/broker-icon.png"),
				}}, nil),
				WithSelectBrokerReturn(&proto.SBResponse{SessionId: "started-session-id"}, nil),
				WithEndSessionReturn(nil),
			),
			args:             &proto.ESRequest{SessionId: "started-session-id"},
			selectedBrokerID: "test-broker",
		},

		// Error cases
		"Error with nil args and empty options": {
			client:    NewDummyClient(nil),
			wantError: errors.New("no input values provided"),
		},
		"Error with empty args empty options": {
			client:    NewDummyClient(nil),
			args:      &proto.ESRequest{},
			wantError: errors.New("no session ID provided"),
		},
		"Error with not-matching session ID": {
			client:    NewDummyClient(nil),
			args:      &proto.ESRequest{SessionId: "a-session-id"},
			wantError: errors.New(`impossible to end session "a-session-id", not found`),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dc, ok := tc.client.(*DummyClient)
			require.True(t, ok, "Provided client is not a Dummy client")

			if tc.args != nil && tc.args.SessionId != "" && tc.selectedBrokerID != "" {
				ret, err := tc.client.SelectBroker(context.TODO(), &proto.SBRequest{
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

			require.Equal(t, &proto.Empty{}, ret)
			require.Empty(t, dc.CurrentSessionID())
			require.Empty(t, dc.SelectedUsername())
		})
	}
}

func TestSetDefaultBrokerForUser(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		client proto.PAMClient
		args   *proto.SDBFURequest

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
			args: &proto.SDBFURequest{
				BrokerId: "broker-id",
				Username: "username",
			},
		},

		// Error cases
		"Error if no user name is provided": {
			client:    NewDummyClient(nil),
			args:      &proto.SDBFURequest{BrokerId: "broker-id"},
			wantError: errors.New("no valid username provided"),
		},
		"Error if no broker ID is provided": {
			client:    NewDummyClient(nil),
			args:      &proto.SDBFURequest{Username: "username"},
			wantError: errors.New("no valid broker ID provided"),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ret, err := tc.client.SetDefaultBrokerForUser(context.TODO(), tc.args)
			require.Equal(t, err, tc.wantError)
			if err != nil {
				require.Nil(t, ret)
				return
			}

			require.Equal(t, &proto.Empty{}, ret)

			if tc.args == nil {
				return
			}
			retBroker, err := tc.client.GetPreviousBroker(context.TODO(),
				&proto.GPBRequest{Username: tc.args.Username})
			require.NoError(t, err)
			require.Equal(t, tc.args.BrokerId, retBroker.PreviousBroker)
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
