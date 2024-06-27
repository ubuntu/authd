package main_test

import (
	"context"
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
	"github.com/ubuntu/authd/internal/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
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

	passwordAuthID = "password"
	fido1AuthID    = "fidodevice1"
	phoneAck1ID    = "phoneack1"
)

var testPasswordUILayout = authd.UILayout{
	Type:    "form",
	Label:   ptrValue("Gimme your password"),
	Entry:   ptrValue("chars_password"),
	Button:  ptrValue(""),
	Code:    ptrValue(""),
	Content: ptrValue(""),
	Wait:    ptrValue(""),
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
	gpasswdOutput := filepath.Join(t.TempDir(), "gpasswd.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")

	testCases := map[string]struct {
		supportedLayouts   []*authd.UILayout
		pamUser            *string
		protoVersion       uint32
		brokerName         string
		eventPollResponses map[gdm.EventType][]*gdm.EventData

		wantError            error
		wantAuthModeIDs      []string
		wantUILayouts        []*authd.UILayout
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
		},
		"Authenticates user-mfa": {
			pamUser:         ptrValue("user-mfa"),
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
		},
		"Authenticates user-mfa after retry": {
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
			wantPamErrorMessages: []string{
				"can't select broker: rpc error: code = InvalidArgument desc = can't start authentication transaction: rpc error: code = InvalidArgument desc = no user name provided",
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
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_brokersReceived: {
					gdm_test.SelectBrokerEvent("some-unknown-broker"),
				},
			},
			wantPamErrorMessages: []string{
				"Sending GDM event failed: Conversation error",
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
			wantError:       pam.ErrAuth,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			t.Cleanup(pam_test.MaybeDoLeakCheck)

			// We run a daemon for each test, because here we don't want to
			// make assumptions whether the state of the broker and each test
			// should run in parallel and work the same way in any order is ran.
			ctx, cancel := context.WithCancel(context.Background())
			env := append(localgroupstestutils.AuthdIntegrationTestsEnvWithGpasswdMock(t, gpasswdOutput, groupsFile), authdCurrentUserRootEnvVariableContent)
			socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath, testutils.WithEnvironment(env...))
			t.Cleanup(func() {
				cancel()
				<-stopped
			})
			moduleArgs := []string{"socket=" + socketPath}

			gdmLog := prepareFileLogging(t, "authd-pam-gdm.log")
			t.Cleanup(func() { saveArtifactsForDebug(t, []string{gdmLog}) })
			moduleArgs = append(moduleArgs, "debug=true", "logfile="+gdmLog)

			serviceFile := createServiceFile(t, "gdm-authd", libPath,
				moduleArgs)

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
			case <-time.After(30 * time.Second):
				timedOut = true
				t.Fatal("Authentication timed out!")
			case err = <-authResult:
			}

			require.ErrorIs(t, err, tc.wantError, "PAM Error does not match expected")
			require.Equal(t, tc.wantPamErrorMessages, gh.pamErrorMessages,
				"PAM Error messages do not match")
			require.Equal(t, tc.wantPamInfoMessages, gh.pamInfoMessages,
				"PAM Info messages do not match")

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
	moduleArgs := []string{libPath}

	gpasswdOutput := filepath.Join(t.TempDir(), "gpasswd.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")
	ctx, cancel := context.WithCancel(context.Background())
	env := append(localgroupstestutils.AuthdIntegrationTestsEnvWithGpasswdMock(t, gpasswdOutput, groupsFile), authdCurrentUserRootEnvVariableContent)
	socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath, testutils.WithEnvironment(env...))
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	moduleArgs = append(moduleArgs, "socket="+socketPath)

	gdmLog := prepareFileLogging(t, "authd-pam-gdm.log")
	t.Cleanup(func() { saveArtifactsForDebug(t, []string{gdmLog}) })
	moduleArgs = append(moduleArgs, "debug=true", "logfile="+gdmLog)

	serviceFile := createServiceFile(t, "gdm-authd", libPath, moduleArgs)
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
	moduleArgs := []string{libPath}

	gpasswdOutput := filepath.Join(t.TempDir(), "gpasswd.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")
	ctx, cancel := context.WithCancel(context.Background())
	env := append(localgroupstestutils.AuthdIntegrationTestsEnvWithGpasswdMock(t, gpasswdOutput, groupsFile), authdCurrentUserRootEnvVariableContent)
	socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath, testutils.WithEnvironment(env...))
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	moduleArgs = append(moduleArgs, "socket="+socketPath)

	gdmLog := prepareFileLogging(t, "authd-pam-gdm.log")
	t.Cleanup(func() { saveArtifactsForDebug(t, []string{gdmLog}) })
	moduleArgs = append(moduleArgs, "debug=true", "logfile="+gdmLog)

	serviceFile := createServiceFile(t, "gdm-authd", libPath, moduleArgs)
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
	cmd.Args = append(cmd.Args, "-buildmode=c-shared", "-gcflags=-dwarflocationlists=true")
	cmd.Env = append(os.Environ(), `CGO_CFLAGS=-O0 -g3`)
	if pam_test.IsAddressSanitizerActive() {
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
