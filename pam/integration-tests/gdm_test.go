package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/brokers/auth"
	"github.com/ubuntu/authd/internal/brokers/layouts"
	"github.com/ubuntu/authd/internal/brokers/layouts/entries"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/pam/internal/gdm"
	"github.com/ubuntu/authd/pam/internal/gdm_test"
	"github.com/ubuntu/authd/pam/internal/pam_test"
	"github.com/ubuntu/authd/pam/internal/proto"
)

func enableGdmExtension() {
	gdm.AdvertisePamExtensions([]string{gdm.PamExtensionCustomJSON})
}

func init() {
	enableGdmExtension()
}

const (
	exampleBrokerName = "ExampleBroker"
	ignoredBrokerName = "<ignored-broker>"

	passwordAuthID           = "password"
	newPasswordAuthID        = "mandatoryreset"
	fido1AuthID              = "fidodevice1"
	phoneAck1ID              = "phoneack1"
	qrcodeID                 = "qrcodeandcodewithtypo"
	qrcodeWithoutCodeID      = "qrcodewithtypo"
	qrcodeWithoutRenderingID = "codewithtypo"
)

var testPasswordUILayout = authd.UILayout{
	Type:    layouts.Form,
	Label:   ptrValue("Gimme your password"),
	Entry:   ptrValue(entries.CharsPassword),
	Button:  ptrValue(""),
	Code:    ptrValue(""),
	Content: ptrValue(""),
	Wait:    ptrValue(""),
}

var testNewPasswordUILayout = authd.UILayout{
	Type:    layouts.NewPassword,
	Label:   ptrValue("Enter your new password"),
	Entry:   ptrValue(entries.CharsPassword),
	Button:  ptrValue(""),
	Code:    ptrValue(""),
	Content: ptrValue(""),
	Wait:    ptrValue(""),
}

var testQrcodeUILayout = authd.UILayout{
	Type:    layouts.QrCode,
	Label:   ptrValue("Scan the qrcode or enter the code in the login page"),
	Content: ptrValue("https://ubuntu.com"),
	Wait:    ptrValue("true"),
	Button:  ptrValue("Regenerate code"),
	Code:    ptrValue("1337"),
	Entry:   ptrValue(""),
}

var testQrcodeUIWithoutCodeLayout = authd.UILayout{
	Type:    layouts.QrCode,
	Label:   ptrValue("Enter the following code after flashing the address: 1337"),
	Content: ptrValue("https://ubuntu.com"),
	Wait:    ptrValue("true"),
	Button:  ptrValue("Regenerate code"),
	Code:    ptrValue(""),
	Entry:   ptrValue(""),
}

var testQrcodeUIWithoutRendering = authd.UILayout{
	Type:    layouts.QrCode,
	Label:   ptrValue("Enter the code in the login page"),
	Content: ptrValue("https://ubuntu.com"),
	Wait:    ptrValue("true"),
	Button:  ptrValue("Regenerate code"),
	Code:    ptrValue("1337"),
	Entry:   ptrValue(""),
}

var testFidoDeviceUILayout = authd.UILayout{
	Type:    layouts.Form,
	Label:   ptrValue("Plug your fido device and press with your thumb"),
	Content: ptrValue(""),
	Wait:    ptrValue("true"),
	Button:  ptrValue(""),
	Code:    ptrValue(""),
	Entry:   ptrValue(""),
}

var testPhoneAckUILayout = authd.UILayout{
	Type:    layouts.Form,
	Label:   ptrValue("Unlock your phone +33... or accept request on web interface:"),
	Content: ptrValue(""),
	Wait:    ptrValue("true"),
	Button:  ptrValue(""),
	Code:    ptrValue(""),
	Entry:   ptrValue(""),
}

func TestGdmModule(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	if !pam.CheckPamHasStartConfdir() {
		t.Fatal("can't test with this libpam version!")
	}

	require.True(t, pam.CheckPamHasBinaryProtocol(),
		"PAM does not support binary protocol")

	libPath := buildPAMModule(t)
	socketPath := runAuthd(t, os.DevNull, os.DevNull, true)

	testCases := map[string]struct {
		supportedLayouts   []*authd.UILayout
		pamUser            *string
		protoVersion       uint32
		brokerName         string
		eventPollResponses map[gdm.EventType][]*gdm.EventData

		wantError            error
		wantAuthModeIDs      []string
		wantUILayouts        []*authd.UILayout
		wantAuthResponses    []*authd.IAResponse
		wantPamInfoMessages  []string
		wantPamErrorMessages []string
		wantAcctMgmtErr      error
	}{
		"Authenticates user": {
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
				},
			},
		},
		"Authenticates user with multiple retries": {
			wantAuthModeIDs: []string{passwordAuthID, passwordAuthID, passwordAuthID},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "not goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpasssss",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
				},
			},
			wantAuthResponses: []*authd.IAResponse{
				{
					Access: auth.Retry,
					Msg:    "invalid password 'not goodpass', should be 'goodpass'",
				},
				{
					Access: auth.Retry,
					Msg:    "invalid password 'goodpasssss', should be 'goodpass'",
				},
				{Access: auth.Granted},
			},
		},
		"Authenticates with MFA": {
			pamUser:         ptrValue("user-mfa-integration-basic"),
			wantAuthModeIDs: []string{passwordAuthID, fido1AuthID, phoneAck1ID},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{
				&testPasswordUILayout,
				&testFidoDeviceUILayout,
				&testPhoneAckUILayout,
			},
			wantAuthResponses: []*authd.IAResponse{
				{Access: auth.Next},
				{Access: auth.Next},
				{Access: auth.Granted},
			},
		},
		"Authenticates user with MFA after retry": {
			pamUser:         ptrValue("user-mfa-integration-retry"),
			wantAuthModeIDs: []string{passwordAuthID, passwordAuthID, fido1AuthID, phoneAck1ID},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "not goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{
				&testPasswordUILayout,
				&testPasswordUILayout,
				&testFidoDeviceUILayout,
				&testPhoneAckUILayout,
			},
			wantAuthResponses: []*authd.IAResponse{
				{
					Access: auth.Retry,
					Msg:    "invalid password 'not goodpass', should be 'goodpass'",
				},
				{Access: auth.Next},
				{Access: auth.Next},
				{Access: auth.Granted},
			},
		},
		"Authenticates user switching to phone ack": {
			wantAuthModeIDs: []string{passwordAuthID, phoneAck1ID},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.EventsGroupBegin(),
					gdm_test.ChangeStageEvent(proto.Stage_authModeSelection),
					gdm_test.AuthModeSelectedEvent(phoneAck1ID),
					gdm_test.EventsGroupEnd(),

					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{
				&testPasswordUILayout,
				&testPhoneAckUILayout,
			},
			wantAuthResponses: []*authd.IAResponse{
				{Access: auth.Cancelled},
				{Access: auth.Granted},
			},
		},
		"Authenticates after password change": {
			pamUser:         ptrValue("user-needs-reset-integration-gdm-pass"),
			wantAuthModeIDs: []string{passwordAuthID, newPasswordAuthID},
			supportedLayouts: []*authd.UILayout{
				pam_test.FormUILayout(),
				pam_test.NewPasswordUILayout(),
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "authd2404",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{&testPasswordUILayout, &testNewPasswordUILayout},
			wantAuthResponses: []*authd.IAResponse{
				{Access: auth.Next},
				{Access: auth.Granted},
			},
		},
		"Authenticates after mfa authentication with wait and password change checking quality": {
			pamUser: ptrValue("user-mfa-needs-reset-integration-gdm-wait-and-new-password"),
			wantAuthModeIDs: []string{
				passwordAuthID,
				fido1AuthID,
				newPasswordAuthID,
				newPasswordAuthID,
				newPasswordAuthID,
				newPasswordAuthID,
			},
			supportedLayouts: []*authd.UILayout{
				pam_test.FormUILayout(),
				pam_test.NewPasswordUILayout(),
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					// Login with password
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
					// Authenticate with fido device
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
					// Use bad dictionary password
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "password",
					}),
					// Use password not meeting broker criteria
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "noble2404",
					}),
					// Use previous one
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
					// Finally change the password
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "authd2404",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{
				&testPasswordUILayout,
				&testFidoDeviceUILayout,
				&testNewPasswordUILayout,
				&testNewPasswordUILayout,
				&testNewPasswordUILayout,
				&testNewPasswordUILayout,
			},
			wantAuthResponses: []*authd.IAResponse{
				{Access: auth.Next},
				{Access: auth.Next},
				{
					Access: auth.Retry,
					Msg:    "The password fails the dictionary check - it is based on a dictionary word",
				},
				{
					Access: auth.Retry,
					Msg:    "new password does not match criteria: must be 'authd2404'",
				},
				{
					Access: auth.Retry,
					Msg:    "The password is the same as the old one",
				},
				{Access: auth.Granted},
			},
		},
		"Authenticates after various invalid password changes": {
			pamUser: ptrValue("user-needs-reset-integration-gdm-retries"),
			wantAuthModeIDs: []string{
				passwordAuthID,
				newPasswordAuthID,
				newPasswordAuthID,
				newPasswordAuthID,
				newPasswordAuthID,
				newPasswordAuthID,
			},
			supportedLayouts: []*authd.UILayout{
				pam_test.FormUILayout(),
				pam_test.NewPasswordUILayout(),
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "authd",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "password",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "newpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "authd2404",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{&testPasswordUILayout, &testNewPasswordUILayout},
			wantAuthResponses: []*authd.IAResponse{
				{
					Access: auth.Next,
				},
				{
					Access: auth.Retry,
					Msg:    "The password is shorter than 8 characters",
				},
				{
					Access: auth.Retry,
					Msg:    "The password is the same as the old one",
				},
				{
					Access: auth.Retry,
					Msg:    "The password fails the dictionary check - it is based on a dictionary word",
				},
				{
					Access: auth.Retry,
					Msg:    "The password is shorter than 8 characters",
				},
				{
					Access: auth.Granted,
				},
			},
		},
		"Authenticates user with qrcode": {
			wantAuthModeIDs:  []string{qrcodeID},
			supportedLayouts: []*authd.UILayout{pam_test.QrCodeUILayout()},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{&testQrcodeUILayout},
		},
		"Authenticates user with qrcode without code field": {
			wantAuthModeIDs: []string{qrcodeWithoutCodeID},
			supportedLayouts: []*authd.UILayout{
				pam_test.QrCodeUILayout(pam_test.WithQrCodeCode("")),
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{&testQrcodeUIWithoutCodeLayout},
		},
		"Authenticates user with qrcode without rendering support": {
			wantAuthModeIDs: []string{qrcodeWithoutRenderingID},
			supportedLayouts: []*authd.UILayout{
				pam_test.QrCodeUILayout(pam_test.WithQrCodeRenders(ptrValue(false))),
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{&testQrcodeUIWithoutRendering},
		},
		"Authenticates user with qrcode without explicit rendering support": {
			// This checks that we're backward compatible
			wantAuthModeIDs: []string{qrcodeID},
			supportedLayouts: []*authd.UILayout{
				pam_test.QrCodeUILayout(pam_test.WithQrCodeRenders(nil)),
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{&testQrcodeUILayout},
		},
		"Authenticates user after switching to qrcode": {
			wantAuthModeIDs: []string{passwordAuthID, qrcodeID},
			supportedLayouts: []*authd.UILayout{
				pam_test.FormUILayout(),
				pam_test.QrCodeUILayout(),
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.EventsGroupBegin(),
					gdm_test.ChangeStageEvent(proto.Stage_authModeSelection),
					gdm_test.AuthModeSelectedEvent(qrcodeID),
					gdm_test.EventsGroupEnd(),

					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{
				&testPasswordUILayout,
				&testQrcodeUILayout,
			},
			wantAuthResponses: []*authd.IAResponse{
				{Access: auth.Cancelled},
				{Access: auth.Granted},
			},
		},
		//nolint:dupl // This is not a duplicate test, parameters are different!
		"Authenticates user after regenerating the qrcode with optional code field": {
			wantAuthModeIDs: []string{
				passwordAuthID,
				qrcodeID,
				qrcodeID,
				qrcodeID,
				qrcodeID,
				qrcodeID,
				qrcodeID,
			},
			supportedLayouts: []*authd.UILayout{
				pam_test.FormUILayout(),
				pam_test.QrCodeUILayout(pam_test.WithQrCodeCode("optional")),
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.EventsGroupBegin(),
					gdm_test.ChangeStageEvent(proto.Stage_authModeSelection),
					gdm_test.AuthModeSelectedEvent(qrcodeID),
					gdm_test.EventsGroupEnd(),

					// Start authentication and regenerate the qrcode (1)
					gdm_test.EventsGroupBegin(),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
					gdm_test.ReselectAuthMode(),
					gdm_test.EventsGroupEnd(),

					// Only regenerate the qr code (2)
					gdm_test.ReselectAuthMode(),

					// Start authentication and regenerate the qrcode (3)
					gdm_test.EventsGroupBegin(),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
					gdm_test.ReselectAuthMode(),
					gdm_test.EventsGroupEnd(),

					// Only regenerate the qr code (4)
					gdm_test.ReselectAuthMode(),

					// Start the final authentication (5)
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{
				&testPasswordUILayout,
				testQrcodeUILayoutData(0),
				testQrcodeUILayoutData(1),
				testQrcodeUILayoutData(2),
				testQrcodeUILayoutData(3),
				testQrcodeUILayoutData(4),
				testQrcodeUILayoutData(5),
			},
			wantAuthResponses: []*authd.IAResponse{
				{Access: auth.Cancelled},
				{Access: auth.Cancelled},
				{Access: auth.Cancelled},
				{Access: auth.Granted},
			},
		},
		//nolint:dupl // This is not a duplicate test, parameters are different!
		"Authenticates user after regenerating the qrcode without code field": {
			wantAuthModeIDs: []string{
				passwordAuthID,
				qrcodeWithoutCodeID,
				qrcodeWithoutCodeID,
				qrcodeWithoutCodeID,
				qrcodeWithoutCodeID,
				qrcodeWithoutCodeID,
				qrcodeWithoutCodeID,
			},
			supportedLayouts: []*authd.UILayout{
				pam_test.FormUILayout(),
				pam_test.QrCodeUILayout(pam_test.WithQrCodeCode("")),
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.EventsGroupBegin(),
					gdm_test.ChangeStageEvent(proto.Stage_authModeSelection),
					gdm_test.AuthModeSelectedEvent(qrcodeWithoutCodeID),
					gdm_test.EventsGroupEnd(),

					// Start authentication and regenerate the qrcode (1)
					gdm_test.EventsGroupBegin(),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
					gdm_test.ReselectAuthMode(),
					gdm_test.EventsGroupEnd(),

					// Only regenerate the qr code (2)
					gdm_test.ReselectAuthMode(),

					// Start authentication and regenerate the qrcode (3)
					gdm_test.EventsGroupBegin(),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
					gdm_test.ReselectAuthMode(),
					gdm_test.EventsGroupEnd(),

					// Only regenerate the qr code (4)
					gdm_test.ReselectAuthMode(),

					// Start the final authentication (5)
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
			},
			wantUILayouts: []*authd.UILayout{
				&testPasswordUILayout,
				testQrcodeWithoutCodeUILayoutData(0),
				testQrcodeWithoutCodeUILayoutData(1),
				testQrcodeWithoutCodeUILayoutData(2),
				testQrcodeWithoutCodeUILayoutData(3),
				testQrcodeWithoutCodeUILayoutData(4),
				testQrcodeWithoutCodeUILayoutData(5),
			},
			wantAuthResponses: []*authd.IAResponse{
				{Access: auth.Cancelled},
				{Access: auth.Cancelled},
				{Access: auth.Cancelled},
				{Access: auth.Granted},
			},
		},

		// Error cases
		"Error on unknown protocol": {
			protoVersion: 9999,
			wantPamErrorMessages: []string{
				"GDM protocol initialization failed, type hello, version 9999",
			},
			wantError:       pam.ErrCredUnavail,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
		"Error on missing user": {
			pamUser: ptrValue(""),
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_brokersReceived: {
					gdm_test.SelectBrokerEvent(exampleBrokerName),
				},
			},
			wantPamErrorMessages: []string{
				"can't select broker: error InvalidArgument from server: can't start authentication transaction: rpc error: code = InvalidArgument desc = no user name provided",
			},
			wantError:       pam.ErrSystem,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
		"Error on no supported layouts": {
			supportedLayouts: []*authd.UILayout{},
			wantPamErrorMessages: []string{
				"UI does not support any layouts",
			},
			wantError:       pam.ErrCredUnavail,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
		"Error on unknown broker": {
			brokerName: "Not a valid broker!",
			wantPamErrorMessages: []string{
				"Changing GDM stage failed: Conversation error",
			},
			wantError:       pam.ErrSystem,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
		"Error (ignored) on local broker causes fallback error": {
			brokerName: brokers.LocalBrokerName,
			wantPamInfoMessages: []string{
				"auth=incomplete",
			},
			wantError:       pam_test.ErrIgnore,
			wantAcctMgmtErr: pam.ErrAbort,
		},
		"Error on authenticating user with too many retries": {
			wantAuthModeIDs: []string{
				passwordAuthID,
				passwordAuthID,
				passwordAuthID,
				passwordAuthID,
				passwordAuthID,
				passwordAuthID,
			},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "not goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "another not goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "even more not goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "not yet goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "really, it's not a goodpass!",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
				},
			},
			wantAuthResponses: []*authd.IAResponse{
				{
					Access: auth.Retry,
					Msg:    "invalid password 'not goodpass', should be 'goodpass'",
				},
				{
					Access: auth.Retry,
					Msg:    "invalid password 'another not goodpass', should be 'goodpass'",
				},
				{
					Access: auth.Retry,
					Msg:    "invalid password 'even more not goodpass', should be 'goodpass'",
				},
				{
					Access: auth.Retry,
					Msg:    "invalid password 'not yet goodpass', should be 'goodpass'",
				},
				{
					Access: auth.Denied,
					Msg:    "invalid password 'really, it's not a goodpass!', should be 'goodpass'",
				},
			},
			wantPamErrorMessages: []string{
				"invalid password 'really, it's not a goodpass!', should be 'goodpass'",
			},
			wantError:       pam.ErrAuth,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
		"Error on authenticating unknown user": {
			pamUser: ptrValue("user-unknown"),
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "",
					}),
				},
			},
			wantPamErrorMessages: []string{
				"user not found",
			},
			wantAuthResponses: []*authd.IAResponse{
				{
					Access: auth.Denied,
					Msg:    "user not found",
				},
			},
			wantError:       pam.ErrAuth,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
		"Error on invalid fido ack": {
			pamUser:         ptrValue("user-mfa-integration-error-fido-ack"),
			wantAuthModeIDs: []string{passwordAuthID, fido1AuthID},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{}),
				},
			},
			wantPamErrorMessages: []string{
				fido1AuthID + " should have wait set to true",
			},
			wantUILayouts: []*authd.UILayout{
				&testPasswordUILayout,
				&testFidoDeviceUILayout,
			},
			wantAuthResponses: []*authd.IAResponse{
				{Access: auth.Next},
				{
					Access: auth.Denied,
					Msg:    fido1AuthID + " should have wait set to true",
				},
			},
			wantError:       pam.ErrAuth,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			moduleArgs := []string{"socket=" + socketPath}

			gdmLog := prepareFileLogging(t, "authd-pam-gdm.log")
			moduleArgs = append(moduleArgs, "debug=true", "logfile="+gdmLog)

			serviceFile := createServiceFile(t, "gdm-authd", libPath, moduleArgs)
			saveArtifactsForDebugOnCleanup(t, []string{serviceFile})

			pamUser := "user-integration-" + strings.ReplaceAll(filepath.Base(t.Name()), "_", "-")
			if tc.pamUser != nil {
				pamUser = *tc.pamUser
			}

			timedOut := false
			gh := newGdmTestModuleHandler(t, serviceFile, pamUser)
			t.Cleanup(func() {
				if !timedOut {
					require.NoError(t, gh.tx.End(), "PAM: can't end transaction")
				}
			})
			gh.eventPollResponses = tc.eventPollResponses

			gh.supportedLayouts = tc.supportedLayouts
			if tc.supportedLayouts == nil {
				gh.supportedLayouts = []*authd.UILayout{pam_test.FormUILayout()}
			}

			gh.protoVersion = gdm.ProtoVersion
			if tc.protoVersion != 0 {
				gh.protoVersion = tc.protoVersion
			}

			gh.selectedBrokerName = tc.brokerName
			if gh.selectedBrokerName == "" {
				gh.selectedBrokerName = exampleBrokerName
			}

			gh.selectedAuthModeIDs = tc.wantAuthModeIDs
			if gh.selectedAuthModeIDs == nil {
				gh.selectedAuthModeIDs = []string{passwordAuthID}
			}

			gh.selectedUILayouts = tc.wantUILayouts
			if gh.selectedAuthModeIDs == nil &&
				len(gh.selectedAuthModeIDs) == 1 &&
				gh.selectedAuthModeIDs[0] == passwordAuthID {
				gh.selectedUILayouts = []*authd.UILayout{&testPasswordUILayout}
			}

			if tc.wantError == nil && tc.wantAuthResponses == nil && len(gh.selectedAuthModeIDs) == 1 {
				tc.wantAuthResponses = []*authd.IAResponse{{Access: auth.Granted}}
			}

			var pamFlags pam.Flags
			if !testutils.IsVerbose() {
				pamFlags = pam.Silent
			}

			authResult := make(chan error)
			go func() {
				authResult <- gh.tx.Authenticate(pamFlags)
			}()

			var err error
			select {
			case <-time.After(sleepDuration(30 * time.Second)):
				timedOut = true
				t.Fatal("Authentication timed out!")
			case err = <-authResult:
			}

			require.ErrorIs(t, err, tc.wantError, "PAM Error does not match expected")
			require.Equal(t, tc.wantPamErrorMessages, gh.pamErrorMessages,
				"PAM Error messages do not match")
			require.Equal(t, tc.wantPamInfoMessages, gh.pamInfoMessages,
				"PAM Info messages do not match")
			gdm_test.RequireEqualData(t, tc.wantAuthResponses, gh.authResponses,
				"Authentication responses do not match")

			requirePreviousBrokerForUser(t, socketPath, "", pamUser)

			require.ErrorIs(t, gh.tx.AcctMgmt(pamFlags), tc.wantAcctMgmtErr,
				"Account Management PAM Error messages do not match")

			if tc.wantError != nil {
				requirePreviousBrokerForUser(t, socketPath, "", pamUser)
				return
			}

			user, err := gh.tx.GetItem(pam.User)
			require.NoError(t, err, "Can't get the pam user")
			require.Equal(t, pamUser, user, "PAM user name does not match expected")

			requirePreviousBrokerForUser(t, socketPath, gh.selectedBrokerName, user)
		})
	}
}

func TestGdmModuleAuthenticateWithoutGdmExtension(t *testing.T) {
	// This cannot be parallel!
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	libPath := buildPAMModule(t)
	moduleArgs := []string{}

	socketPath := runAuthd(t, os.DevNull, os.DevNull, true)
	moduleArgs = append(moduleArgs, "socket="+socketPath)

	gdmLog := prepareFileLogging(t, "authd-pam-gdm.log")
	moduleArgs = append(moduleArgs, "debug=true", "logfile="+gdmLog)

	serviceFile := createServiceFile(t, "gdm-authd", libPath, moduleArgs)
	saveArtifactsForDebugOnCleanup(t, []string{serviceFile})
	pamUser := "user-integration-auth-no-gdm-extension"
	gh := newGdmTestModuleHandler(t, serviceFile, pamUser)
	t.Cleanup(func() { require.NoError(t, gh.tx.End(), "PAM: can't end transaction") })

	// We disable gdm extension support, as if it was the case when the module is loaded
	// outside GDM.
	gdm.AdvertisePamExtensions(nil)
	t.Cleanup(enableGdmExtension)

	var pamFlags pam.Flags
	if !testutils.IsVerbose() {
		pamFlags = pam.Silent
	}

	require.ErrorIs(t, gh.tx.Authenticate(pamFlags), pam_test.ErrIgnore,
		"Authentication should be ignored")
	requirePreviousBrokerForUser(t, socketPath, "", pamUser)
}

func TestGdmModuleAcctMgmtWithoutGdmExtension(t *testing.T) {
	// This cannot be parallel!
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	libPath := buildPAMModule(t)
	moduleArgs := []string{}

	socketPath := runAuthd(t, os.DevNull, os.DevNull, true)
	moduleArgs = append(moduleArgs, "socket="+socketPath)

	gdmLog := prepareFileLogging(t, "authd-pam-gdm.log")
	moduleArgs = append(moduleArgs, "debug=true", "logfile="+gdmLog)

	serviceFile := createServiceFile(t, "gdm-authd", libPath, moduleArgs)
	saveArtifactsForDebugOnCleanup(t, []string{serviceFile})
	pamUser := "user-integration-acctmgmt-no-gdm-extension"
	gh := newGdmTestModuleHandler(t, serviceFile, pamUser)
	t.Cleanup(func() { require.NoError(t, gh.tx.End(), "PAM: can't end transaction") })

	gh.supportedLayouts = []*authd.UILayout{pam_test.FormUILayout()}
	gh.protoVersion = gdm.ProtoVersion
	gh.selectedBrokerName = exampleBrokerName
	gh.selectedAuthModeIDs = []string{passwordAuthID}
	gh.eventPollResponses = map[gdm.EventType][]*gdm.EventData{
		gdm.EventType_startAuthentication: {
			gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
				Challenge: "goodpass",
			}),
		},
	}

	var pamFlags pam.Flags
	if !testutils.IsVerbose() {
		pamFlags = pam.Silent
	}

	require.NoError(t, gh.tx.Authenticate(pamFlags), "Setup: Authentication failed")
	requirePreviousBrokerForUser(t, socketPath, "", pamUser)

	// We disable gdm extension support, as if it was the case when the module is loaded
	// again from the exec module.
	gdm.AdvertisePamExtensions(nil)
	t.Cleanup(enableGdmExtension)

	require.ErrorIs(t, gh.tx.AcctMgmt(pamFlags), pam_test.ErrIgnore,
		"Account Management PAM Error message do not match")
	requirePreviousBrokerForUser(t, socketPath, "", pamUser)
}

func buildPAMModule(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "build", "-C", "..")
	if testutils.CoverDirForTests() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
	}
	// FIXME: This leads to an EOM error when loading the compiled module:
	// if testutils.IsRace() {
	// 	cmd.Args = append(cmd.Args, "-race")
	// }
	cmd.Args = append(cmd.Args, "-buildmode=c-shared")
	cmd.Args = append(cmd.Args, "-gcflags=all=-N -l")
	cmd.Env = append(os.Environ(), `CGO_CFLAGS=-O0 -g3`)
	if testutils.IsAsan() {
		cmd.Args = append(cmd.Args, "-asan")
	}

	libPath := filepath.Join(t.TempDir(), "libpam_authd.so")
	t.Logf("Compiling PAM library at %s", libPath)

	cmd.Args = append(cmd.Args, "-tags=pam_debug,pam_gdm_debug", "-o", libPath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: could not compile PAM module: %s", out)
	if string(out) != "" {
		t.Log(string(out))
	}

	return libPath
}

func exampleBrokerQrcodeData(reqN int) (string, string) {
	// Keep this in sync with example broker's qrcodeData
	baseCode := 1337
	qrcodeURIs := []string{
		"https://ubuntu.com",
		"https://ubuntu.fr/",
		"https://ubuntuforum-br.org/",
		"https://www.ubuntu-it.org/",
	}

	return qrcodeURIs[reqN%len(qrcodeURIs)], fmt.Sprint(baseCode + reqN)
}

func testQrcodeUILayoutData(reqN int) *authd.UILayout {
	content, code := exampleBrokerQrcodeData(reqN)
	base := &testQrcodeUILayout
	return &authd.UILayout{
		Type:    base.Type,
		Label:   base.Label,
		Content: &content,
		Wait:    base.Wait,
		Button:  base.Button,
		Code:    &code,
		Entry:   base.Entry,
	}
}

func testQrcodeWithoutCodeUILayoutData(reqN int) *authd.UILayout {
	content, code := exampleBrokerQrcodeData(reqN)
	base := &testQrcodeUIWithoutCodeLayout
	return &authd.UILayout{
		Type:    base.Type,
		Label:   ptrValue("Enter the following code after flashing the address: " + code),
		Content: &content,
		Wait:    base.Wait,
		Button:  base.Button,
		Code:    base.Code,
		Entry:   base.Entry,
	}
}
