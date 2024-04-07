package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/testutils"
	grouptests "github.com/ubuntu/authd/internal/users/localgroups/tests"
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
	localBrokerName   = "local"
	ignoredBrokerName = "<ignored-broker>"

	passwordAuthID = "password"
	fido1AuthID    = "fidodevice1"
	phoneAck1ID    = "phoneack1"
)

//nolint:thelper // This is actually a test!
func testGdmModule(t *testing.T, libPath string, args []string) {
	if !pam.CheckPamHasStartConfdir() {
		t.Fatal("can't test with this libpam version!")
	}

	require.True(t, pam.CheckPamHasBinaryProtocol(),
		"PAM does not support binary protocol")

	gpasswdOutput := filepath.Join(t.TempDir(), "gpasswd.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")

	testCases := map[string]struct {
		supportedLayouts   []*authd.UILayout
		pamUser            string
		protoVersion       uint32
		brokerName         string
		authModeIDs        []string
		eventPollResponses map[gdm.EventType][]*gdm.EventData

		wantError            error
		wantPamInfoMessages  []string
		wantPamErrorMessages []string
		wantAcctMgmtErr      error
	}{
		"Authenticates user1": {
			pamUser: "user1",
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Challenge{
						Challenge: "goodpass",
					}),
				},
			},
		},
		"Authenticates user2 with multiple retries": {
			pamUser:     "user2",
			authModeIDs: []string{passwordAuthID, passwordAuthID, passwordAuthID},
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
			pamUser:     "user-mfa",
			authModeIDs: []string{passwordAuthID, fido1AuthID, phoneAck1ID},
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
			pamUser:     "user-mfa",
			authModeIDs: []string{passwordAuthID, passwordAuthID, fido1AuthID, phoneAck1ID},
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
		"Authenticates user2 after switching to phone ack": {
			pamUser:     "user2",
			authModeIDs: []string{passwordAuthID, phoneAck1ID},
			eventPollResponses: map[gdm.EventType][]*gdm.EventData{
				gdm.EventType_startAuthentication: {
					gdm_test.ChangeStageEvent(proto.Stage_authModeSelection),
					gdm_test.IsAuthenticatedEvent(&authd.IARequest_AuthenticationData_Wait{
						Wait: "true",
					}),
				},
				gdm.EventType_authEvent: {
					gdm_test.AuthModeSelectedEvent(phoneAck1ID),
				},
			},
		},

		// Error cases
		"Error on unknown protocol": {
			pamUser:      "user-foo",
			protoVersion: 9999,
			wantPamErrorMessages: []string{
				"GDM protocol initialization failed, type hello, version 9999",
			},
			wantError:       pam.ErrCredUnavail,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
		"Error on missing user": {
			pamUser: "",
			wantPamErrorMessages: []string{
				"can't select broker: rpc error: code = InvalidArgument desc = can't start authentication transaction: rpc error: code = InvalidArgument desc = no user name provided",
			},
			wantError:       pam.ErrSystem,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
		"Error on no supported layouts": {
			pamUser:          "user-bar",
			supportedLayouts: []*authd.UILayout{},
			wantPamErrorMessages: []string{
				"UI does not support any layouts",
			},
			wantError:       pam.ErrCredUnavail,
			wantAcctMgmtErr: pam_test.ErrIgnore,
		},
		"Error on unknown broker": {
			pamUser:    "user-foo",
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
			pamUser:    "user-foo",
			brokerName: localBrokerName,
			wantPamInfoMessages: []string{
				"auth=incomplete",
			},
			wantError:       pam_test.ErrIgnore,
			wantAcctMgmtErr: pam.ErrAbort,
		},
		"Error on authenticating user2 with too many retries": {
			pamUser: "user2",
			authModeIDs: []string{
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
			pamUser: "user-unknown",
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
			pamUser:     "user-mfa",
			authModeIDs: []string{passwordAuthID, fido1AuthID},
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

			moduleArgs := slices.Clone(args)

			// We run a daemon for each test, because here we don't want to
			// make assumptions whether the state of the broker and each test
			// should run in parallel and work the same way in any order is ran.
			ctx, cancel := context.WithCancel(context.Background())
			socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath,
				testutils.WithEnvironment(grouptests.GPasswdMockEnv(t, gpasswdOutput, groupsFile)...),
			)
			t.Cleanup(func() {
				cancel()
				<-stopped
			})
			moduleArgs = append(moduleArgs, "socket="+socketPath)

			gdmLog := prepareFileLogging(t, "authd-pam-gdm.log")
			t.Cleanup(func() { saveArtifactsForDebug(t, []string{gdmLog}) })
			moduleArgs = append(moduleArgs, "debug=true", "logfile="+gdmLog)

			serviceFile := createServiceFile(t, "gdm-authd", libPath,
				moduleArgs)

			timedOut := false
			gh := newGdmTestModuleHandler(t, serviceFile, tc.pamUser)
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

			gh.selectedAuthModeIDs = tc.authModeIDs
			if gh.selectedAuthModeIDs == nil {
				gh.selectedAuthModeIDs = []string{passwordAuthID}
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

			requirePreviousBrokerForUser(t, socketPath, "", tc.pamUser)

			require.ErrorIs(t, gh.tx.AcctMgmt(pamFlags), tc.wantAcctMgmtErr,
				"Account Management PAM Error messages do not match")

			if tc.wantError != nil {
				requirePreviousBrokerForUser(t, socketPath, "", tc.pamUser)
				return
			}

			user, err := gh.tx.GetItem(pam.User)
			require.NoError(t, err, "Can't get the pam user")
			require.Equal(t, tc.pamUser, user, "PAM user name does not match expected")

			requirePreviousBrokerForUser(t, socketPath, gh.selectedBrokerName, user)
		})
	}
}

func TestGdmModule(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	libPath := buildPAMModule(t)
	testGdmModule(t, libPath, nil)
}

func TestGdmModuleWithExecModule(t *testing.T) {
	t.Parallel()
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	execLib, moduleArgs := prepareGdmModuleTestsWithExecModule(t)
	testGdmModule(t, execLib, moduleArgs)
}

func prepareGdmModuleTestsWithExecModule(t *testing.T) (string, []string) {
	t.Helper()

	execLib := buildExecModule(t)
	cliPath := buildPAMClient(t)
	moduleArgs := []string{"--exec-debug"}
	if !testutils.IsVerbose() {
		logFile := prepareFileLogging(t, "exec-module.log")
		saveArtifactsForDebug(t, []string{logFile})
		moduleArgs = append(moduleArgs, "--exec-log", logFile)
	}
	if env := testutils.CoverDirEnv(); env != "" {
		moduleArgs = append(moduleArgs, "--exec-env", testutils.CoverDirEnv())
	}
	moduleArgs = append(moduleArgs, "--", cliPath)

	return execLib, moduleArgs
}

//nolint:thelper // This is actually a test!
func testGdmModuleAuthenticateWithoutGdmExtension(t *testing.T, libPath string, moduleArgs []string) {
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	gpasswdOutput := filepath.Join(t.TempDir(), "gpasswd.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")
	ctx, cancel := context.WithCancel(context.Background())
	socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath,
		testutils.WithEnvironment(grouptests.GPasswdMockEnv(t, gpasswdOutput, groupsFile)...))
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	moduleArgs = append(moduleArgs, "socket="+socketPath)

	gdmLog := prepareFileLogging(t, "authd-pam-gdm.log")
	t.Cleanup(func() { saveArtifactsForDebug(t, []string{gdmLog}) })
	moduleArgs = append(moduleArgs, "debug=true", "logfile="+gdmLog)

	serviceFile := createServiceFile(t, "gdm-authd", libPath, moduleArgs)
	pamUser := "user1"
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

	require.ErrorIs(t, gh.tx.Authenticate(pamFlags), pam.ErrSystem,
		"Authentication should fail")
	requirePreviousBrokerForUser(t, socketPath, "", pamUser)
}

func TestGdmModuleAuthenticateWithoutGdmExtension(t *testing.T) {
	// This cannot be parallel!

	testGdmModuleAuthenticateWithoutGdmExtension(t, buildPAMModule(t), nil)
}

func TestGdmModuleAuthenticateWithoutGdmExtensionWithExecModule(t *testing.T) {
	// This cannot be parallel!

	execLib, moduleArgs := prepareGdmModuleTestsWithExecModule(t)
	testGdmModuleAuthenticateWithoutGdmExtension(t, execLib, moduleArgs)
}

//nolint:thelper // This is actually a test!
func testGdmModuleAcctMgmtWithoutGdmExtension(t *testing.T, libPath string, moduleArgs []string) {
	// This cannot be parallel!
	t.Cleanup(pam_test.MaybeDoLeakCheck)

	gpasswdOutput := filepath.Join(t.TempDir(), "gpasswd.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")
	ctx, cancel := context.WithCancel(context.Background())
	socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath,
		testutils.WithEnvironment(grouptests.GPasswdMockEnv(t, gpasswdOutput, groupsFile)...))
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	moduleArgs = append(moduleArgs, "socket="+socketPath)

	gdmLog := prepareFileLogging(t, "authd-pam-gdm.log")
	t.Cleanup(func() { saveArtifactsForDebug(t, []string{gdmLog}) })
	moduleArgs = append(moduleArgs, "debug=true", "logfile="+gdmLog)

	serviceFile := createServiceFile(t, "gdm-authd", libPath, moduleArgs)
	pamUser := "user1"
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

func TestGdmModuleAcctMgmtWithoutGdmExtension(t *testing.T) {
	// This cannot be parallel!
	testGdmModuleAcctMgmtWithoutGdmExtension(t, buildPAMModule(t), nil)
}

func TestGdmModuleAcctMgmtWithoutGdmExtensionWithExecModule(t *testing.T) {
	// This cannot be parallel!

	execLib, moduleArgs := prepareGdmModuleTestsWithExecModule(t)
	testGdmModuleAcctMgmtWithoutGdmExtension(t, execLib, moduleArgs)
}

func buildPAMModule(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "build", "-C", "..")
	if testutils.CoverDir() != "" {
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
