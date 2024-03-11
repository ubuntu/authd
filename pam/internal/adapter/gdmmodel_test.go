package adapter

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/pam/internal/gdm"
	"github.com/ubuntu/authd/pam/internal/gdm_test"
	"github.com/ubuntu/authd/pam/internal/pam_test"
	"github.com/ubuntu/authd/pam/internal/proto"
	pam_proto "github.com/ubuntu/authd/pam/internal/proto"
)

var gdmTestPrivateKey *rsa.PrivateKey
var gdmTestSequentialMessages atomic.Int64

const gdmTestIgnoredMessage string = "<ignored>"

func TestGdmModel(t *testing.T) {
	t.Parallel()

	// This is not technically an error, as it means that during the tests
	// we've stopped the program with a Quit request.
	// However we do return a PAM error in such case because that's what we're
	// going to return to the PAM stack in case authentication process has not
	// been completed fully.
	gdmTestEarlyStopExitStatus := pamError{
		status: pam.ErrSystem,
		msg:    "model did not return anything",
	}

	gdmTestIgnoreStage := pam_proto.Stage(-1)

	firstBrokerInfo := &authd.ABResponse_BrokerInfo{
		Id:        "testBroker",
		Name:      "The best broker!",
		BrandIcon: nil,
	}
	secondBrokerInfo := &authd.ABResponse_BrokerInfo{
		Id:        "secondaryBroker",
		Name:      "A broker that works too!",
		BrandIcon: nil,
	}

	passwordUILayoutID := "Password"
	singleBrokerClientOptions := []pam_test.DummyClientOptions{
		// FIXME: Ideally we should use proper ID checks, but this can currently lead to
		// races because the way our model is implemented and events can't be stopped and may
		// arrive con delay.
		pam_test.WithIgnoreSessionIDChecks(),
		pam_test.WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
			firstBrokerInfo,
		}, nil),
		pam_test.WithUILayout(passwordUILayoutID, "Password authentication", pam_test.FormUILayout()),
	}
	multiBrokerClientOptions := append(slices.Clone(singleBrokerClientOptions),
		pam_test.WithAvailableBrokers([]*authd.ABResponse_BrokerInfo{
			firstBrokerInfo, secondBrokerInfo,
		}, nil),
	)

	testCases := map[string]struct {
		client           authd.PAMClient
		clientOptions    []pam_test.DummyClientOptions
		supportedLayouts []*authd.UILayout
		messages         []tea.Msg
		commands         []tea.Cmd
		gdmEvents        []*gdm.EventData
		pamUser          string
		protoVersion     uint32
		convError        map[string]error

		wantExitStatus     PamReturnStatus
		wantGdmRequests    []gdm.RequestType
		wantGdmEvents      []gdm.EventType
		wantGdmAuthRes     []*authd.IAResponse
		wantNoGdmRequests  []gdm.RequestType
		wantNoGdmEvents    []gdm.EventType
		wantNoBrokers      bool
		wantSelectedBroker string
		wantStage          pam_proto.Stage
		wantUsername       string
		wantMessages       []tea.Msg
	}{
		"User selection stage": {
			wantGdmRequests: []gdm.RequestType{gdm.RequestType_uiLayoutCapabilities},
			wantStage:       pam_proto.Stage_userSelection,
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_brokersReceived,
			},
			wantNoGdmRequests: []gdm.RequestType{
				gdm.RequestType_changeStage, // -> broker Selection
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Broker selection stage caused by server-side user selection": {
			messages:     []tea.Msg{userSelected{username: "daemon-selected-user"}},
			wantUsername: "daemon-selected-user",
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_brokerSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Broker selection stage caused by server-side user selection after broker": {
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_userSelection,
					commands: []tea.Cmd{
						tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
							return userSelected{username: "daemon-selected-user"}
						}),
						sendEvent(gdmTestWaitForStage{stage: pam_proto.Stage_brokerSelection}),
					},
				},
			},
			wantUsername: "daemon-selected-user",
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_brokerSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Broker selection stage caused by client-side user selection": {
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user"),
			},
			wantUsername: "gdm-selected-user",
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_brokerSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Broker selection stage caused by module user selection": {
			pamUser:      "gdm-pam-selected-user",
			wantUsername: "gdm-pam-selected-user",
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_brokerSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Challenge stage caused by server-side broker and authMode selection": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil)),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker"}
				}))(),
				gdmTestWaitForStage{stage: pam_proto.Stage_challenge},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_challenge,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Challenge stage caused by client-side broker and authMode selection": {
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user-and-broker"),
			},
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{stage: pam_proto.Stage_challenge},
			},
			wantUsername:       "gdm-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_challenge,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Authenticated after server-side user, broker and authMode selection": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithIsAuthenticatedWantChallenge("gdm-good-password")),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency*2, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker"}
				}))(),
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-good-password",
						}}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthGranted}},
			wantStage:      pam_proto.Stage_challenge,
			wantExitStatus: PamSuccess{BrokerID: firstBrokerInfo.Id},
		},
		"Authenticated with message after server-side user, broker and authMode selection": {
			clientOptions: append(slices.Clone(multiBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithIsAuthenticatedReturn(&authd.IAResponse{
					Access: brokers.AuthGranted,
					Msg:    `{"message": "Hi GDM, it's a pleasure to get you in!"}`,
				}, nil),
			),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency*2, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker"}
				}))(),
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-good-password",
						}}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_authEvent,
				gdm.EventType_startAuthentication,
			},
			wantStage: pam_proto.Stage_challenge,
			wantGdmAuthRes: []*authd.IAResponse{{
				Access: brokers.AuthGranted,
				Msg:    "Hi GDM, it's a pleasure to get you in!",
			}},
			wantExitStatus: PamSuccess{
				BrokerID: firstBrokerInfo.Id,
				msg:      "Hi GDM, it's a pleasure to get you in!",
			},
		},
		"Authentication is ignored if not requested by model first": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithIsAuthenticatedWantChallenge("gdm-good-password")),
			gdmEvents: []*gdm.EventData{
				gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
					Challenge: "gdm-good-password",
				}),
			},
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			wantNoGdmRequests: []gdm.RequestType{
				gdm.RequestType_changeStage,
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_brokersReceived,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_userSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Cancelled after server-side user, broker and authMode selection": {
			clientOptions: append(slices.Clone(multiBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithIsAuthenticatedReturn(&authd.IAResponse{
					Access: brokers.AuthCancelled,
				}, nil),
			),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency*2, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker"}
				}))(),
				gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
					Challenge: "gdm-any-password",
				}},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_challenge,
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthCancelled}},
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Explicitly cancelled after server-side user, broker and authMode selection": {
			clientOptions: append(slices.Clone(multiBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
			),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency*2, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker"}
				}))(),
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					events: []*gdm.EventData{
						gdm_test.IsAuthenticatedCancelledEvent(),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_challenge,
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthCancelled}},
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Authenticated after server-side user, broker and authMode selection and after various retries": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithIsAuthenticatedWantChallenge("gdm-good-password"),
				pam_test.WithIsAuthenticatedMaxRetries(1),
			),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency*2, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker"}
				}))(),
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-bad-password",
						}}),
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-good-password",
						}}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent, // retry
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent, // denied
			},
			wantMessages: []tea.Msg{
				startAuthentication{},
				startAuthentication{},
			},
			wantGdmAuthRes: []*authd.IAResponse{
				{Access: brokers.AuthRetry},
				{Access: brokers.AuthGranted},
			},
			wantStage:      pam_proto.Stage_challenge,
			wantExitStatus: PamSuccess{BrokerID: firstBrokerInfo.Id},
		},
		"Authenticated after client-side user, broker and authMode selection": {
			clientOptions: append(slices.Clone(multiBrokerClientOptions),
				pam_test.WithIsAuthenticatedWantChallenge("gdm-good-password"),
			),
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user-broker-and-auth-mode"),
			},
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(secondBrokerInfo.Id),
					},
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-good-password",
						}}),
					},
				},
			},
			wantUsername:       "gdm-selected-user-broker-and-auth-mode",
			wantSelectedBroker: secondBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage:      pam_proto.Stage_challenge,
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthGranted}},
			wantExitStatus: PamSuccess{BrokerID: secondBrokerInfo.Id},
		},
		"Authenticated after client-side user, broker and authMode selection and after various retries": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithIsAuthenticatedWantChallenge("gdm-good-password"),
				pam_test.WithIsAuthenticatedMaxRetries(1),
			),
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user-broker-and-auth-mode"),
			},
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-bad-password",
						}}),
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-good-password",
						}}),
					},
				},
			},
			wantUsername:       "gdm-selected-user-broker-and-auth-mode",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantMessages: []tea.Msg{
				startAuthentication{},
				startAuthentication{},
			},
			wantGdmAuthRes: []*authd.IAResponse{
				{Access: brokers.AuthRetry},
				{Access: brokers.AuthGranted},
			},
			wantStage:      pam_proto.Stage_challenge,
			wantExitStatus: PamSuccess{BrokerID: firstBrokerInfo.Id},
		},
		"Cancelled auth after client-side user, broker and authMode selection": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithIsAuthenticatedReturn(&authd.IAResponse{
					Access: brokers.AuthCancelled,
				}, nil),
			),
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user-broker-and-auth-mode"),
			},
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
					Challenge: "gdm-some-password",
				}},
			},
			wantUsername:       "gdm-selected-user-broker-and-auth-mode",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantMessages: []tea.Msg{
				startAuthentication{},
			},
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthCancelled}},
			wantStage:      pam_proto.Stage_challenge,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"AuthMode selection stage from client after server-side broker and auth mode selection if there is only one auth mode": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
			),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker"}
				}))(),
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					events: []*gdm.EventData{
						gdm_test.ChangeStageEvent(pam_proto.Stage_authModeSelection),
					},
					commands: []tea.Cmd{
						sendEvent(gdmTestWaitForStage{stage: pam_proto.Stage_authModeSelection}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
				gdm.RequestType_changeStage, // -> authMode Selection
			},
			wantMessages: []tea.Msg{
				startAuthentication{},
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_authEvent,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_authEvent,
			},
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthCancelled}},
			wantStage:      pam_proto.Stage_authModeSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"AuthMode selection stage from client after server-side broker and auth mode selection with multiple auth modes": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithUILayout("pincode", "Pin Code", pam_test.FormUILayout()),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-and-broker"},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					events: []*gdm.EventData{
						gdm_test.ChangeStageEvent(pam_proto.Stage_authModeSelection),
					},
					commands: []tea.Cmd{
						sendEvent(gdmTestWaitForStage{stage: pam_proto.Stage_authModeSelection}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
				gdm.RequestType_changeStage, // -> authMode Selection
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthCancelled}},
			wantStage:      pam_proto.Stage_authModeSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"AuthMode selection stage from client after client-side broker and auth mode selection if there is only one auth mode": {
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user-broker-and-auth-mode"),
			},
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					events: []*gdm.EventData{
						gdm_test.ChangeStageEvent(pam_proto.Stage_authModeSelection),
					},
					commands: []tea.Cmd{
						sendEvent(gdmTestWaitForStage{stage: pam_proto.Stage_authModeSelection}),
					},
				},
			},
			wantUsername:       "gdm-selected-user-broker-and-auth-mode",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
				gdm.RequestType_changeStage, // -> authMode Selection
			},
			wantMessages: []tea.Msg{
				startAuthentication{},
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthCancelled}},
			wantStage:      pam_proto.Stage_authModeSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"Authenticated after auth selection stage from client after client-side broker and auth mode selection if there is only one auth mode": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithIsAuthenticatedWantChallenge("gdm-good-password"),
			),
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user-broker-and-auth-mode"),
			},
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					events: []*gdm.EventData{
						gdm_test.ChangeStageEvent(pam_proto.Stage_authModeSelection),
					},
					commands: []tea.Cmd{
						sendEvent(gdmTestWaitForStage{
							stage: pam_proto.Stage_authModeSelection,
							events: []*gdm.EventData{
								gdm_test.AuthModeSelectedEvent(passwordUILayoutID),
							},
							commands: []tea.Cmd{
								sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
									Challenge: "gdm-good-password",
								}}),
							},
						}),
					},
				},
			},
			wantUsername:       "gdm-selected-user-broker-and-auth-mode",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantMessages: []tea.Msg{
				startAuthentication{},
				startAuthentication{},
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage: pam_proto.Stage_challenge,
			wantGdmAuthRes: []*authd.IAResponse{
				{Access: brokers.AuthCancelled},
				{Access: brokers.AuthGranted},
			},
			wantExitStatus: PamSuccess{BrokerID: firstBrokerInfo.Id},
		},
		"Authenticated after auth selection stage from client after client-side broker and auth mode selection with multiple auth modes": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithUILayout("pincode", "Write the pin Code", pam_test.FormUILayout()),
				pam_test.WithIsAuthenticatedWantChallenge("1234"),
			),
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user-broker-and-auth-mode"),
			},
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					events: []*gdm.EventData{
						gdm_test.ChangeStageEvent(pam_proto.Stage_authModeSelection),
					},
					commands: []tea.Cmd{
						sendEvent(gdmTestWaitForStage{
							stage: pam_proto.Stage_authModeSelection,
							events: []*gdm.EventData{
								gdm_test.AuthModeSelectedEvent("pincode"),
							},
							commands: []tea.Cmd{
								sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
									Challenge: "1234",
								}}),
							},
						}),
					},
				},
			},
			wantUsername:       "gdm-selected-user-broker-and-auth-mode",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantMessages: []tea.Msg{
				startAuthentication{},
				startAuthentication{},
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_startAuthentication,
				gdm.EventType_authEvent,
			},
			wantStage: pam_proto.Stage_challenge,
			wantGdmAuthRes: []*authd.IAResponse{
				{Access: brokers.AuthCancelled},
				{Access: brokers.AuthGranted},
			},
			wantExitStatus: PamSuccess{BrokerID: firstBrokerInfo.Id},
		},
		"Broker selection stage from client after client-side broker and auth mode selection if there is only one auth mode": {
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user-broker-and-auth-mode"),
			},
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					events: []*gdm.EventData{
						gdm_test.ChangeStageEvent(pam_proto.Stage_authModeSelection),
					},
					commands: []tea.Cmd{
						sendEvent(gdmTestWaitForStage{
							stage: pam_proto.Stage_authModeSelection,
							events: []*gdm.EventData{
								gdm_test.ChangeStageEvent(pam_proto.Stage_brokerSelection),
							},
						}),
					},
				},
			},
			wantUsername:       "gdm-selected-user-broker-and-auth-mode",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> broker Selection
			},
			wantMessages: []tea.Msg{
				startAuthentication{},
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_authEvent,
			},
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthCancelled}},
			wantStage:      pam_proto.Stage_brokerSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},
		"User selection stage from client after client-side broker and auth mode selection if there is only one auth mode": {
			gdmEvents: []*gdm.EventData{
				gdm_test.SelectUserEvent("gdm-selected-user-broker-and-auth-mode"),
			},
			messages: []tea.Msg{
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					events: []*gdm.EventData{
						gdm_test.ChangeStageEvent(pam_proto.Stage_authModeSelection),
					},
					commands: []tea.Cmd{
						sendEvent(gdmTestWaitForStage{
							stage: pam_proto.Stage_authModeSelection,
							events: []*gdm.EventData{
								gdm_test.ChangeStageEvent(pam_proto.Stage_brokerSelection),
							},
							commands: []tea.Cmd{
								sendEvent(gdmTestWaitForStage{
									stage: pam_proto.Stage_brokerSelection,
									events: []*gdm.EventData{
										gdm_test.ChangeStageEvent(pam_proto.Stage_userSelection),
									},
									commands: []tea.Cmd{
										sendEvent(gdmTestWaitForStage{stage: pam_proto.Stage_userSelection}),
									},
								}),
							},
						}),
					},
				},
			},
			wantUsername:       "gdm-selected-user-broker-and-auth-mode",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> user Selection
			},
			wantMessages: []tea.Msg{
				startAuthentication{},
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModeSelected,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_authEvent,
			},
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthCancelled}},
			wantStage:      pam_proto.Stage_userSelection,
			wantExitStatus: gdmTestEarlyStopExitStatus,
		},

		// Error cases
		"Error on no UI layouts": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithUILayout(passwordUILayoutID, "", &authd.UILayout{}),
			),
			supportedLayouts: []*authd.UILayout{},
			wantGdmRequests:  []gdm.RequestType{gdm.RequestType_uiLayoutCapabilities},
			wantGdmEvents:    []gdm.EventType{gdm.EventType_brokersReceived},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
			},
			wantExitStatus: pamError{
				status: pam.ErrCredUnavail,
				msg:    "UI does not support any layouts",
			},
		},
		"Error on brokers fetching error": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithAvailableBrokers(nil, errors.New("brokers loading failed")),
			),
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_brokersReceived,
				gdm.EventType_userSelected,
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "could not get current available brokers: brokers loading failed",
			},
			wantNoBrokers: true,
		},
		"Error on forced quit": {
			messages:       []tea.Msg{tea.Quit()},
			wantExitStatus: gdmTestEarlyStopExitStatus,
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
			},
		},
		"Error on invalid poll data response for missing type": {
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			gdmEvents: []*gdm.EventData{
				{
					Type: gdm.EventType_userSelected,
				},
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_brokersReceived,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "Sending GDM poll failed: Conversation error: poll response data member 0 invalid: missing event data",
			},
		},
		"Error on invalid poll data response for missing data": {
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			gdmEvents: []*gdm.EventData{{}},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_brokersReceived,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "Sending GDM poll failed: Conversation error: poll response data member 0 invalid: missing event type",
			},
		},
		"Error on no brokers": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithAvailableBrokers(nil, nil),
			),
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_brokersReceived,
				gdm.EventType_userSelected,
			},
			wantExitStatus: pamError{
				status: pam.ErrAuthinfoUnavail,
				msg:    "No brokers available",
			},
		},
		"Error on invalid broker selection": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithSelectBrokerReturn(nil, errors.New("error during broker selection")),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-and-broker"},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "can't select broker: error during broker selection",
			},
		},
		"Error during broker selection if session ID is empty": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithIgnoreSessionIDGeneration(),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithSelectBrokerReturn(&authd.SBResponse{}, nil),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-and-broker"},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "no session ID returned by broker",
			},
		},
		"Error during broker selection if encryption key is empty": {
			client: pam_test.NewDummyClient(nil, append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithSelectBrokerReturn(&authd.SBResponse{SessionId: "session-id"}, nil),
			)...),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-and-broker"},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "no encryption key returned by broker",
			},
		},
		"Error during broker selection if encryption key is not valid base64": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithSelectBrokerReturn(&authd.SBResponse{
					SessionId:     "session-id",
					EncryptionKey: "no encryption key returned by broker",
				}, nil),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-and-broker"},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "encryption key sent by broker is not a valid base64 encoded string: illegal base64 data at input byte 2",
			},
		},
		"Error during broker selection if encryption key is not valid key": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithSelectBrokerReturn(&authd.SBResponse{
					SessionId: "session-id",
					EncryptionKey: base64.StdEncoding.EncodeToString(
						[]byte("not a valid encryption key!")),
				}, nil),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-and-broker"},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    gdmTestIgnoredMessage,
			},
		},
		"Error during broker auth mode selection if UI is not valid": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithUILayout(passwordUILayoutID, "", nil),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-for-client-selected-broker"},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{stage: proto.Stage_authModeSelection},
			},
			wantUsername:       "daemon-selected-user-for-client-selected-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
			},
			wantStage: gdmTestIgnoreStage,
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "invalid empty UI Layout information from broker",
			},
		},
		"Error on missing authentication modes": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetAuthenticationModesReturn([]*authd.GAMResponse_AuthenticationMode{}, nil),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-for-client-selected-broker"},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{stage: proto.Stage_authModeSelection},
			},
			wantUsername:       "daemon-selected-user-for-client-selected-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
			},
			wantStage: gdmTestIgnoreStage,
			wantExitStatus: pamError{
				status: pam.ErrCredUnavail,
				msg:    "no supported authentication mode available for this provider",
			},
		},
		"Error on authentication mode selection": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithSelectAuthenticationModeReturn(nil, errors.New("error selecting auth mode")),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-for-client-selected-broker"},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{stage: proto.Stage_authModeSelection},
			},
			wantUsername:       "daemon-selected-user-for-client-selected-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
			},
			wantStage: gdmTestIgnoreStage,
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "can't select authentication mode: error selecting auth mode",
			},
		},
		"Error on invalid auth-mode layout type": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithSelectAuthenticationModeReturn(&authd.UILayout{
					Type: "invalid layout",
				}, nil),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-with-client-selected-broker"},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{stage: proto.Stage_challenge},
			},
			wantUsername:       "daemon-selected-user-with-client-selected-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
			},
			wantStage: gdmTestIgnoreStage,
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    `Sending GDM event failed: Conversation error: unknown layout type: "invalid layout"`,
			},
		},
		"Error on authentication client failure": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithIsAuthenticatedReturn(nil, errors.New("some authentication error")),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-for-client-selected-broker"},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
					Challenge: "gdm-password",
				}},
			},
			wantUsername:       "daemon-selected-user-for-client-selected-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
			},
			wantStage: pam_proto.Stage_challenge,
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "authentication status failure: some authentication error",
			},
		},
		"Error on authentication client invalid message": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithIsAuthenticatedReturn(&authd.IAResponse{
					Access: brokers.AuthDenied,
					Msg:    "invalid JSON",
				}, nil),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-and-broker"},
				gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
					Challenge: "gdm-good-password",
				}},
			},
			wantUsername:       "daemon-selected-user-and-broker",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
			},
			wantStage: gdmTestIgnoreStage,
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "invalid json data from provider: invalid character 'i' looking for beginning of value",
			},
		},
		"Error on authentication client denied because of wrong password, with error message": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithIsAuthenticatedWantChallenge("gdm-good-password"),
				pam_test.WithIsAuthenticatedMessage("you're not allowed!"),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-for-client-selected-brokers-with-wrong-pass"},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_authModeSelection,
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-wrong-password",
						}}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-for-client-selected-brokers-with-wrong-pass",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_authEvent,
			},
			wantStage: gdmTestIgnoreStage,
			wantGdmAuthRes: []*authd.IAResponse{{
				Access: brokers.AuthDenied,
				Msg:    "you're not allowed!",
			}},
			wantExitStatus: pamError{
				status: pam.ErrAuth,
				msg:    "you're not allowed!",
			},
		},
		"Error on authentication client denied because of wrong password": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithIsAuthenticatedWantChallenge("gdm-good-password"),
			),
			messages: []tea.Msg{
				userSelected{username: "daemon-selected-user-and-client-selected-broker-with-wrong-pass"},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_brokerSelection,
					events: []*gdm.EventData{
						gdm_test.SelectBrokerEvent(firstBrokerInfo.Id),
					},
				},
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-wrong-password",
						}}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-client-selected-broker-with-wrong-pass",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_authEvent,
			},
			wantStage:      gdmTestIgnoreStage,
			wantGdmAuthRes: []*authd.IAResponse{{Access: brokers.AuthDenied}},
			wantExitStatus: pamError{
				status: pam.ErrAuth,
				msg:    "Access denied",
			},
		},
		"Error on authentication client denied because of wrong password after retry": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithIsAuthenticatedWantChallenge("gdm-good-password"),
				pam_test.WithIsAuthenticatedMaxRetries(1),
			),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency*2, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker-with-wrong-pass"}
				}))(),
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-wrong-password",
						}}),
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-another-wrong-password",
						}}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-broker-with-wrong-pass",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_authEvent, // retry
				gdm.EventType_authEvent, // denied
			},
			wantStage: gdmTestIgnoreStage,
			wantGdmAuthRes: []*authd.IAResponse{
				{Access: brokers.AuthRetry},
				{Access: brokers.AuthDenied},
			},
			wantExitStatus: pamError{
				status: pam.ErrAuth,
				msg:    "Access denied",
			},
		},
		"Error on authentication client because of empty auth data access": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithIsAuthenticatedReturn(&authd.IAResponse{}, nil),
			),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency*2, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker-with-wrong-pass"}
				}))(),
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-some-password",
						}}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-broker-with-wrong-pass",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_authEvent, // denied
			},
			wantStage: gdmTestIgnoreStage,
			wantGdmAuthRes: []*authd.IAResponse{{
				Access: brokers.AuthDenied,
				Msg:    `Access "" is not valid`,
			}},
			wantExitStatus: pamError{
				status: pam.ErrAuth,
				msg:    `Access "" is not valid`,
			},
		},
		"Error on authentication client because of invalid auth data access with message": {
			clientOptions: append(slices.Clone(singleBrokerClientOptions),
				pam_test.WithGetPreviousBrokerReturn(firstBrokerInfo.Id, nil),
				pam_test.WithIsAuthenticatedReturn(&authd.IAResponse{
					Access: "no way you get here!",
					Msg:    `{"message": "This is not a valid access"}`,
				}, nil),
			),
			messages: []tea.Msg{
				tea.Sequence(tea.Tick(gdmPollFrequency*2, func(t time.Time) tea.Msg {
					return userSelected{username: "daemon-selected-user-and-broker-with-wrong-pass"}
				}))(),
				gdmTestWaitForStage{
					stage: pam_proto.Stage_challenge,
					commands: []tea.Cmd{
						sendEvent(gdmTestSendAuthDataWhenReady{&authd.IARequest_AuthenticationData_Challenge{
							Challenge: "gdm-some-password",
						}}),
					},
				},
			},
			wantUsername:       "daemon-selected-user-and-broker-with-wrong-pass",
			wantSelectedBroker: firstBrokerInfo.Id,
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
				gdm.RequestType_changeStage, // -> broker Selection
				gdm.RequestType_changeStage, // -> authMode Selection
				gdm.RequestType_changeStage, // -> challenge
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokersReceived,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_authEvent, // denied
			},
			wantStage: gdmTestIgnoreStage,
			wantGdmAuthRes: []*authd.IAResponse{{
				Access: brokers.AuthDenied,
				Msg:    `Access "no way you get here!" is not valid`,
			}},
			wantExitStatus: pamError{
				status: pam.ErrAuth,
				msg:    `Access "no way you get here!" is not valid`,
			},
		},
		"Error on change stage using an unknown stage": {
			gdmEvents: []*gdm.EventData{
				gdm_test.ChangeStageEvent(gdmTestIgnoreStage),
			},
			wantGdmRequests: []gdm.RequestType{
				gdm.RequestType_uiLayoutCapabilities,
			},
			wantNoGdmRequests: []gdm.RequestType{
				gdm.RequestType_changeStage,
			},
			wantGdmEvents: []gdm.EventType{
				gdm.EventType_brokersReceived,
			},
			wantNoGdmEvents: []gdm.EventType{
				gdm.EventType_userSelected,
				gdm.EventType_brokerSelected,
				gdm.EventType_authModesReceived,
				gdm.EventType_authModeSelected,
				gdm.EventType_uiLayoutReceived,
				gdm.EventType_authEvent,
			},
			wantStage: gdmTestIgnoreStage,
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    `unknown PAM stage: "-1"`,
			},
		},
		"Error during hello conversation": {
			convError: map[string]error{
				gdm_test.DataToJSON(t, &gdm.Data{
					Type: gdm.DataType_hello,
				}): errors.New("this is an hello error"),
			},
			wantExitStatus: pamError{
				status: pam.ErrCredUnavail,
				msg:    "GDM initialization failed: Conversation error: this is an hello error",
			},
		},
		"Error during hello on protocol mismatch": {
			protoVersion: 99999999,
			wantExitStatus: pamError{
				status: pam.ErrCredUnavail,
				msg:    "GDM protocol initialization failed, type hello, version 99999999",
			},
		},
		"Error during poll": {
			convError: map[string]error{
				gdm_test.DataToJSON(t, &gdm.Data{Type: gdm.DataType_poll}): errors.New("this is a poll error"),
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "Sending GDM poll failed: Conversation error: this is a poll error",
			},
		},
		"Error on change stage": {
			convError: map[string]error{
				gdm_test.DataToJSON(t, &gdm.Data{
					Type: gdm.DataType_request,
					Request: &gdm.RequestData{
						Type: gdm.RequestType_changeStage,
						Data: &gdm.RequestData_ChangeStage{
							ChangeStage: &gdm.Requests_ChangeStage{
								Stage: proto.Stage_brokerSelection,
							},
						},
					},
				}): errors.New("this is a stage change error"),
			},
			gdmEvents: []*gdm.EventData{
				gdm_test.ChangeStageEvent(pam_proto.Stage_brokerSelection),
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "Changing GDM stage failed: Conversation error: this is a stage change error",
			},
		},
		"Error on request UI capabilities": {
			convError: map[string]error{
				gdm_test.DataToJSON(t, &gdm.Data{
					Type: gdm.DataType_request,
					Request: &gdm.RequestData{
						Type: gdm.RequestType_uiLayoutCapabilities,
						Data: &gdm.RequestData_UiLayoutCapabilities{},
					},
				}): errors.New("this is an UI capabilities request error"),
			},
			wantExitStatus: pamError{
				status: pam.ErrSystem,
				msg:    "Sending GDM UI capabilities Request failed: Conversation error: this is an UI capabilities request error",
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.clientOptions == nil {
				tc.clientOptions = singleBrokerClientOptions
			}
			if tc.client == nil {
				tc.client = pam_test.NewDummyClient(gdmTestPrivateKey, tc.clientOptions...)
			}

			messagesToSend := tc.messages
			messagesToWait := append(tc.messages, tc.wantMessages...)

			if tc.wantExitStatus != gdmTestEarlyStopExitStatus {
				messagesToWait = append(messagesToWait, tc.wantExitStatus)
			}

			gdmMutex := sync.Mutex{}
			gdmHandler := &gdmConvHandler{
				t:                    t,
				mu:                   &gdmMutex,
				protoVersion:         gdm.ProtoVersion,
				convError:            tc.convError,
				currentStageChanged:  *sync.NewCond(&gdmMutex),
				pendingEventsFlushed: make(chan struct{}),
				allRequestsReceived:  make(chan struct{}),
				allEventsReceived:    make(chan struct{}),
				startAuthRequested:   make(chan struct{}),
				supportedLayouts:     tc.supportedLayouts,
				pendingEvents:        tc.gdmEvents,
				wantEvents:           tc.wantGdmEvents,
				wantRequests:         tc.wantGdmRequests,
			}
			uiModel := UIModel{
				PamMTx:     pam_test.NewModuleTransactionDummy(gdmHandler),
				ClientType: Gdm,
				Client:     tc.client,
			}
			appState := gdmTestUIModel{
				UIModel:             uiModel,
				gdmHandler:          gdmHandler,
				wantMessages:        slices.Clone(messagesToWait),
				wantMessagesHandled: make(chan struct{}),
			}

			if tc.supportedLayouts == nil {
				gdmHandler.supportedLayouts = []*authd.UILayout{pam_test.FormUILayout()}
			}

			if tc.protoVersion != 0 {
				gdmHandler.protoVersion = tc.protoVersion
			}

			if tc.pamUser != "" {
				require.NoError(t, uiModel.PamMTx.SetItem(pam.User, tc.pamUser))
			}

			teaOpts, err := TeaHeadlessOptions()
			require.NoError(t, err, "Setup: Can't setup bubble tea options")
			teaOpts = append(teaOpts, tea.WithFilter(appState.filterFunc))
			p := tea.NewProgram(&appState, teaOpts...)
			appState.program = p

			// testHadTimeout := false
			controlDone := make(chan struct{})
			go func() {
				wg := sync.WaitGroup{}
				if len(messagesToWait) > 0 {
					for _, m := range messagesToSend {
						t.Logf("Sent message %#v\n", m)
						p.Send(m)
					}
					wg.Add(1)
					go func() {
						t.Log("Waiting for wantMessagesHandled", messagesToWait)
						<-appState.wantMessagesHandled
						t.Log("DONE waiting for wantMessagesHandled")
						wg.Done()
					}()
				}
				if len(tc.gdmEvents) > 0 {
					wg.Add(1)
					go func() {
						t.Log("Waiting for pendingEventsFlushed")
						<-gdmHandler.pendingEventsFlushed
						t.Log("DONE waiting for pendingEventsFlushed")
						wg.Done()
					}()
				}
				if len(tc.wantGdmRequests) > 0 {
					wg.Add(1)
					go func() {
						t.Log("Waiting for allRequestsReceived")
						<-gdmHandler.allRequestsReceived
						wg.Done()
						t.Log("DONE waiting for allRequestsReceived")
					}()
				}
				if len(tc.wantGdmEvents) > 0 {
					wg.Add(1)
					go func() {
						t.Log("Waiting for allEventsReceived")
						<-gdmHandler.allEventsReceived
						wg.Done()
						t.Log("DONE waiting for allEventsReceived")
					}()
				}

				t.Log("Waiting for expected events")
				waitChan := make(chan struct{})
				go func() {
					wg.Wait()
					close(waitChan)
				}()
				select {
				case <-time.After(5 * time.Second):
				case <-waitChan:
				}
				t.Log("Waiting for events done...")

				// Ensure we've nothing to send back...
				select {
				case <-time.After(gdmPollFrequency * 2):
					// All good, it seems there's nothing coming.
				case <-gdmHandler.pendingEventsFlushed:
				}
				t.Log("Waiting for flushing events done...")

				defer close(controlDone)
				t.Log("Time to quit!")
				appState.programShouldQuit.Store(true)
				p.Send(tea.Quit())
			}()
			_, err = p.Run()
			require.NoError(t, err)

			logStatus := func() {
				appState.mu.Lock()
				defer appState.mu.Unlock()
				gdmHandler.mu.Lock()
				defer gdmHandler.mu.Unlock()

				receivedEventTypes := []gdm.EventType{}
				for _, e := range gdmHandler.receivedEvents {
					receivedEventTypes = append(receivedEventTypes, e.Type)
				}

				t.Log("----------------")
				t.Logf("Remaining msgs: %#v\n", appState.wantMessages)
				t.Log("----------------")
				t.Logf("Received events: %#v\n", receivedEventTypes)
				t.Logf("Wanted events: %#v\n", tc.wantGdmEvents)
				t.Log("----------------")
				t.Logf("Handled requests: %#v\n", gdmHandler.handledRequests)
				t.Logf("Wanted requests: %#v\n", tc.wantGdmRequests)
				t.Log("----------------")
			}

			select {
			case <-time.After(5 * time.Second):
				logStatus()
				t.Fatalf("timeout waiting for test expected results")
			case <-controlDone:
			}

			gdmHandler.mu.Lock()
			defer gdmHandler.mu.Unlock()

			appState.mu.Lock()
			defer appState.mu.Unlock()

			if tc.wantExitStatus.Message() == gdmTestIgnoredMessage {
				switch wantRet := tc.wantExitStatus.(type) {
				case PamReturnError:
					exitErr, ok := appState.ExitStatus().(PamReturnError)
					require.True(t, ok, "exit status should be an error")
					require.Equal(t, wantRet.Status(), exitErr.Status())
				case PamSuccess:
					_, ok := appState.ExitStatus().(PamSuccess)
					require.True(t, ok, "exit status should be a success")
				default:
					t.Fatalf("Unexpected exit status: %v", wantRet)
				}
			} else {
				require.Equal(t, tc.wantExitStatus, appState.ExitStatus())
			}

			require.True(t, appState.gdmModel.conversationsStopped)

			for _, req := range tc.wantNoGdmRequests {
				require.NotContains(t, gdmHandler.handledRequests, req)
			}

			if tc.wantStage != gdmTestIgnoreStage {
				require.Equal(t, tc.wantStage, gdmHandler.currentStage,
					"GDM Stage does not match with expected one")
			}

			for _, req := range tc.wantGdmRequests {
				// We don't do full equal check since we only care about having received
				// the ones explicitly listed.
				require.Contains(t, gdmHandler.handledRequests, req)
			}

			receivedEventTypes := []gdm.EventType{}
			for _, e := range gdmHandler.receivedEvents {
				receivedEventTypes = append(receivedEventTypes, e.Type)
			}
			require.True(t, isSupersetOf(receivedEventTypes, tc.wantGdmEvents),
				"Required events have not been received: %#v vs %#v", tc.wantGdmEvents, receivedEventTypes)

			require.Empty(t, appState.wantMessages, "Wanted messages have not all been processed")

			username, err := appState.PamMTx.GetItem(pam.User)
			require.NoError(t, err)
			require.Equal(t, tc.wantUsername, username)
			gdm_test.RequireEqualData(t, tc.wantGdmAuthRes, gdmHandler.authEvents)

			if r, ok := tc.wantExitStatus.(PamReturnError); ok {
				// If the model exited with error and that matches, we don't
				// care much comparing all the expectations, since the final exit status
				// is matching what we expect.
				switch r.Status() {
				case pam.ErrIgnore, pam.ErrAuth:
				case gdmTestEarlyStopExitStatus.Status():
				default:
					return
				}
			}

			wantBrokers := []*authd.ABResponse_BrokerInfo(nil)
			if !tc.wantNoBrokers {
				availableBrokers, err := appState.Client.AvailableBrokers(context.TODO(), nil)
				require.NoError(t, err)
				wantBrokers = availableBrokers.GetBrokersInfos()
			}

			gdm_test.RequireEqualData(t, wantBrokers, gdmHandler.receivedBrokers)
			require.Equal(t, tc.wantSelectedBroker, gdmHandler.selectedBrokerID)
		})
	}
}

func TestMain(m *testing.M) {
	var err error
	gdmTestPrivateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("could not create an valid rsa key: %v", err))
	}
	defer pam_test.MaybeDoLeakCheck()
	m.Run()
}
