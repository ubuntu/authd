package pam_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/services/pam"
	"github.com/ubuntu/authd/internal/services/permissions"
	"github.com/ubuntu/authd/internal/services/permissions/permissionstests"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/users"
	cachetests "github.com/ubuntu/authd/internal/users/cache/tests"
	grouptests "github.com/ubuntu/authd/internal/users/localgroups/tests"
	usertests "github.com/ubuntu/authd/internal/users/tests"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	brokerManager         *brokers.Manager
	mockBrokerGeneratedID string
)

// Used for TestGetAuthenticationModes and TestSelectAuthenticationMode.
var (
	requiredEntries = "required:entry_type,other_entry_type"
	optionalEntries = "optional:entry_type,other_entry_type"
	optional        = "optional"

	requiredEntry = &authd.UILayout{
		Type:    "required-entry",
		Label:   &optional,
		Button:  &optional,
		Wait:    &optional,
		Entry:   &requiredEntries,
		Content: &optional,
	}
	optionalEntry = &authd.UILayout{
		Type:  "optional-entry",
		Entry: &optionalEntries,
	}
	emptyType = &authd.UILayout{
		Type:  "",
		Entry: &requiredEntries,
	}
)

func TestNewService(t *testing.T) {
	t.Parallel()

	m, err := users.NewManager(t.TempDir())
	require.NoError(t, err, "Setup: could not create user manager")

	pm := permissions.New()
	service := pam.NewService(context.Background(), m, brokerManager, &pm)

	brokers, err := service.AvailableBrokers(context.Background(), &authd.Empty{})
	require.NoError(t, err, "can’t create the service directly")
	require.NotEmpty(t, brokers.BrokersInfos, "Service is created and can query the broker manager")
}

func TestAvailableBrokers(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		currentUserNotRoot bool

		wantErr bool
	}{
		"Success getting available brokers": {},

		"Error when not root": {currentUserNotRoot: true, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pm := newPermissionManager(t, tc.currentUserNotRoot)
			client := newPamClient(t, nil, &pm)

			abResp, err := client.AvailableBrokers(context.Background(), &authd.Empty{})

			if tc.wantErr {
				require.Error(t, err, "AvailableBrokers should return an error, but did not")
				return
			}
			require.NoError(t, err, "AvailableBrokers should not return an error, but did")

			got := abResp.GetBrokersInfos()
			for _, broker := range got {
				broker.Id = broker.Name + "_ID"
			}
			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "AvailableBrokers returned unexpected brokers")
		})
	}
}

func TestGetPreviousBroker(t *testing.T) {
	t.Parallel()

	// Get local user and get it set to local broker
	u, err := user.Current()
	require.NoError(t, err, "Setup: could not fetch current user")
	currentUsername := u.Username

	tests := map[string]struct {
		user string

		currentUserNotRoot bool

		wantBroker string
		wantErr    bool
	}{
		"Success getting previous broker":  {user: "userwithbroker", wantBroker: mockBrokerGeneratedID},
		"For local user, get local broker": {user: currentUsername, wantBroker: brokers.LocalBrokerName},

		"Returns empty when user does not exist":         {user: "nonexistent", wantBroker: ""},
		"Returns empty when user does not have a broker": {user: "userwithoutbroker", wantBroker: ""},
		"Returns empty when broker is not available":     {user: "userwithinactivebroker", wantBroker: ""},

		"Error when not root": {user: "userwithbroker", currentUserNotRoot: true, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			f, err := os.Open(filepath.Join(testutils.TestFamilyPath(t), "get-previous-broker.db"))
			require.NoError(t, err, "Setup: could not open fixture database file")
			defer f.Close()
			d, err := io.ReadAll(f)
			require.NoError(t, err, "Setup: could not read fixture database file")
			d = bytes.ReplaceAll(d, []byte("MOCKBROKERID"), []byte(mockBrokerGeneratedID))
			err = cachetests.DbfromYAML(bytes.NewBuffer(d), cacheDir)
			require.NoError(t, err, "Setup: could not prepare cache database file")

			expiration, err := time.Parse(time.DateOnly, "2004-01-01")
			require.NoError(t, err, "Setup: could not parse time for testing")

			m, err := users.NewManager(cacheDir, users.WithUserExpirationDate(expiration))
			require.NoError(t, err, "Setup: could not create user manager")
			t.Cleanup(func() { _ = m.Stop() })
			pm := newPermissionManager(t, tc.currentUserNotRoot)
			client := newPamClient(t, m, &pm)

			// Get existing entry
			gotResp, err := client.GetPreviousBroker(context.Background(), &authd.GPBRequest{Username: tc.user})

			if tc.wantErr {
				require.Error(t, err, "GetPreviousBroker should return an error, but did not")
				return
			}
			require.NoError(t, err, "GetPreviousBroker should not return an error, but did")

			require.Equal(t, tc.wantBroker, gotResp.GetPreviousBroker(), "GetPreviousBroker should return expected broker")
		})
	}
}

func TestSelectBroker(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		brokerID    string
		username    string
		sessionMode string

		currentUserNotRoot bool

		wantErr bool
	}{
		"Successfully select a broker and creates auth session":   {username: "success"},
		"Successfully select a broker and creates passwd session": {username: "success", sessionMode: "passwd"},

		"Error when not root":                             {username: "success", currentUserNotRoot: true, wantErr: true},
		"Error when username is empty":                    {wantErr: true},
		"Error when mode is empty":                        {sessionMode: "-", wantErr: true},
		"Error when mode does not exist":                  {sessionMode: "does not exist", wantErr: true},
		"Error when brokerID is empty":                    {username: "empty broker", brokerID: "-", wantErr: true},
		"Error when broker does not exist":                {username: "no broker", brokerID: "does not exist", wantErr: true},
		"Error when broker does not provide a session ID": {username: "NS_no_id", wantErr: true},
		"Error when starting the session":                 {username: "NS_error", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pm := newPermissionManager(t, tc.currentUserNotRoot)
			client := newPamClient(t, nil, &pm)

			switch tc.brokerID {
			case "":
				tc.brokerID = mockBrokerGeneratedID
			case "-":
				tc.brokerID = ""
			}

			if tc.username != "" {
				tc.username = t.Name() + testutils.IDSeparator + tc.username
			}

			var sessionMode authd.SessionMode
			switch tc.sessionMode {
			case "":
				sessionMode = authd.SessionMode_AUTH
			case "passwd":
				sessionMode = authd.SessionMode_PASSWD
			case "-":
				sessionMode = authd.SessionMode_UNDEFINED
			}

			sbRequest := &authd.SBRequest{
				BrokerId: tc.brokerID,
				Username: tc.username,
				Mode:     sessionMode,
			}
			sbResp, err := client.SelectBroker(context.Background(), sbRequest)
			if tc.wantErr {
				require.Error(t, err, "SelectBroker should return an error, but did not")
				return
			}
			require.NoError(t, err, "SelectBroker should not return an error, but did")

			got := fmt.Sprintf("ID: %s\nEncryption Key: %s\n",
				strings.ReplaceAll(sbResp.GetSessionId(), tc.brokerID, "BROKER_ID"),
				sbResp.GetEncryptionKey())
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "SelectBroker returned an unexpected response")
		})
	}
}

func TestGetAuthenticationModes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sessionID          string
		supportedUILayouts []*authd.UILayout

		username           string
		currentUserNotRoot bool

		wantErr bool
	}{
		"Successfully get authentication modes":          {},
		"Successfully get multiple authentication modes": {username: "GAM_multiple_modes"},

		"Error when not root":                     {currentUserNotRoot: true, wantErr: true},
		"Error when sessionID is empty":           {sessionID: "-", wantErr: true},
		"Error when passing invalid layout":       {supportedUILayouts: []*authd.UILayout{emptyType}, wantErr: true},
		"Error when sessionID is invalid":         {sessionID: "invalid-session", wantErr: true},
		"Error when getting authentication modes": {username: "GAM_error", wantErr: true},
		"Error when broker returns invalid modes": {username: "GAM_invalid", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pm := newPermissionManager(t, false) // Allow starting the session (current user considered root)
			client := newPamClient(t, nil, &pm)

			switch tc.sessionID {
			case "invalid-session":
			case "-":
				tc.sessionID = ""
			default:
				id := startSession(t, client, tc.username)
				if tc.sessionID == "" {
					tc.sessionID = id
				}
			}

			// Now, set tests permissions for this use case
			permissionstests.SetCurrentRootAsRoot(&pm, !tc.currentUserNotRoot)

			if tc.supportedUILayouts == nil {
				tc.supportedUILayouts = []*authd.UILayout{requiredEntry}
			}

			gamReq := &authd.GAMRequest{
				SessionId:          tc.sessionID,
				SupportedUiLayouts: tc.supportedUILayouts,
			}
			gamResp, err := client.GetAuthenticationModes(context.Background(), gamReq)
			if tc.wantErr {
				require.Error(t, err, "GetAuthenticationModes should return an error, but did not")
				return
			}
			require.NoError(t, err, "GetAuthenticationModes should not return an error, but did")

			got := gamResp.GetAuthenticationModes()
			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "GetAuthenticationModes returned an unexpected response")
		})
	}
}

func TestSelectAuthenticationMode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sessionID string
		authMode  string

		username           string
		supportedUILayouts []*authd.UILayout
		noValidators       bool
		currentUserNotRoot bool

		wantErr bool
	}{
		"Successfully select mode with required value":         {username: "SAM_success_required_entry", supportedUILayouts: []*authd.UILayout{requiredEntry}},
		"Successfully select mode with missing optional value": {username: "SAM_missing_optional_entry", supportedUILayouts: []*authd.UILayout{optionalEntry}},

		// service errors
		"Error when not root":                {username: "SAM_success_required_entry", currentUserNotRoot: true, wantErr: true},
		"Error when sessionID is empty":      {sessionID: "-", wantErr: true},
		"Error when session ID is invalid":   {sessionID: "invalid-session", wantErr: true},
		"Error when no authmode is selected": {sessionID: "no auth mode", authMode: "-", wantErr: true},

		// broker errors
		"Error when selecting invalid auth mode":                     {username: "SAM_error", supportedUILayouts: []*authd.UILayout{requiredEntry}, wantErr: true},
		"Error when broker does not have validators for the session": {username: "does not matter", noValidators: true, wantErr: true},

		/* Layout errors */
		"Error when returns no layout":                     {username: "SAM_no_layout", supportedUILayouts: []*authd.UILayout{requiredEntry}, wantErr: true},
		"Error when returns layout with no type":           {username: "SAM_no_layout_type", supportedUILayouts: []*authd.UILayout{requiredEntry}, wantErr: true},
		"Error when returns layout without required value": {username: "SAM_missing_required_entry", supportedUILayouts: []*authd.UILayout{requiredEntry}, wantErr: true},
		"Error when returns layout with unknown field":     {username: "SAM_unknown_field", supportedUILayouts: []*authd.UILayout{requiredEntry}, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pm := newPermissionManager(t, false) // Allow starting the session (current user considered root)
			client := newPamClient(t, nil, &pm)

			switch tc.sessionID {
			case "invalid-session":
			case "-":
				tc.sessionID = ""
			default:
				id := startSession(t, client, tc.username)
				if tc.sessionID == "" {
					tc.sessionID = id
				}
			}

			switch tc.authMode {
			case "":
				tc.authMode = "some mode"
			case "-":
				tc.authMode = ""
			}

			// If the username does not have a SAM_something, it means we don't care about the broker answer and we don't need the validators.
			if !tc.noValidators && strings.HasPrefix(tc.username, "SAM_") {
				// We need to call GetAuthenticationModes to generate the layout validators on the broker.
				gamReq := &authd.GAMRequest{
					SessionId:          tc.sessionID,
					SupportedUiLayouts: tc.supportedUILayouts,
				}
				_, err := client.GetAuthenticationModes(context.Background(), gamReq)
				require.NoError(t, err, "Setup: failed to get authentication modes for tests")
			}

			// Now, set tests permissions for this use case
			permissionstests.SetCurrentRootAsRoot(&pm, !tc.currentUserNotRoot)

			samReq := &authd.SAMRequest{
				SessionId:            tc.sessionID,
				AuthenticationModeId: tc.authMode,
			}
			samResp, err := client.SelectAuthenticationMode(context.Background(), samReq)
			if tc.wantErr {
				require.Error(t, err, "SelectAuthenticationMode should return an error, but did not")
				return
			}
			require.NoError(t, err, "SelectAuthenticationMode should not return an error, but did")

			got := samResp.GetUiLayoutInfo()
			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "SelectAuthenticationMode should have returned the expected UI layout")
		})
	}
}

func TestIsAuthenticated(t *testing.T) {
	tests := map[string]struct {
		sessionID  string
		existingDB string

		username           string
		secondCall         bool
		cancelFirstCall    bool
		localGroupsFile    string
		currentUserNotRoot bool

		// There is no wantErr as it's stored in the golden file.
	}{
		"Successfully authenticate":                           {username: "success"},
		"Successfully authenticate if first call is canceled": {username: "IA_second_call", secondCall: true, cancelFirstCall: true},
		"Denies authentication when broker times out":         {username: "IA_timeout"},
		"Update existing DB on success":                       {username: "success", existingDB: "cache-with-user.db"},
		"Update local groups":                                 {username: "success_with_local_groups", localGroupsFile: "valid.group"},

		// service errors
		"Error when not root":           {username: "success", currentUserNotRoot: true},
		"Error when sessionID is empty": {sessionID: "-"},
		"Error when there is no broker": {sessionID: "invalid-session"},

		// broker errors
		"Error when authenticating":                                          {username: "IA_error"},
		"Error on empty data even if granted":                                {username: "IA_empty_data"},
		"Error when broker returns invalid access":                           {username: "IA_invalid_access"},
		"Error when broker returns invalid data":                             {username: "IA_invalid_data"},
		"Error when broker returns invalid userinfo":                         {username: "IA_invalid_userinfo"},
		"Error when broker returns username different than the one selected": {username: "IA_info_mismatching_user_name"},
		"Error when calling second time without cancelling":                  {username: "IA_second_call", secondCall: true},

		// local group error
		"Error on updating local groups with unexisting file": {username: "success_with_local_groups", localGroupsFile: "does_not_exists.group"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.localGroupsFile == "" {
				t.Parallel()
			}

			var destCmdsFile string
			if tc.localGroupsFile != "" {
				destCmdsFile = grouptests.SetupGPasswdMock(t, filepath.Join(testutils.TestFamilyPath(t), tc.localGroupsFile))
			}

			cacheDir := t.TempDir()
			if tc.existingDB != "" {
				f, err := os.Open(filepath.Join(testutils.TestFamilyPath(t), tc.existingDB))
				require.NoError(t, err, "Setup: could not open fixture database file")
				defer f.Close()
				err = cachetests.DbfromYAML(f, cacheDir)
				require.NoError(t, err, "Setup: could not prepare cache database file")
			}

			expiration, err := time.Parse(time.DateOnly, "2004-01-01")
			require.NoError(t, err, "Setup: could not parse time for testing")

			m, err := users.NewManager(cacheDir, users.WithUserExpirationDate(expiration))
			require.NoError(t, err, "Setup: could not create user manager")
			t.Cleanup(func() { _ = m.Stop() })
			pm := newPermissionManager(t, false) // Allow starting the session (current user considered root)
			client := newPamClient(t, m, &pm)

			switch tc.sessionID {
			case "invalid-session":
			case "-":
				tc.sessionID = ""
			default:
				id := startSession(t, client, tc.username)
				if tc.sessionID == "" {
					tc.sessionID = id
				}
			}

			// Now, set tests permissions for this use case
			permissionstests.SetCurrentRootAsRoot(&pm, !tc.currentUserNotRoot)

			var firstCall, secondCall string
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				iaReq := &authd.IARequest{
					SessionId:          tc.sessionID,
					AuthenticationData: &authd.IARequest_AuthenticationData{},
				}
				iaResp, err := client.IsAuthenticated(ctx, iaReq)
				firstCall = fmt.Sprintf("FIRST CALL:\n\taccess: %s\n\tmsg: %s\n\terr: %v\n",
					iaResp.GetAccess(),
					iaResp.GetMsg(),
					err,
				)
			}()
			// Give some time for the first call to block
			time.Sleep(time.Second)
			if tc.cancelFirstCall {
				cancel()
				time.Sleep(500 * time.Millisecond)
				<-done
			}

			if tc.secondCall {
				iaReq := &authd.IARequest{
					SessionId:          tc.sessionID,
					AuthenticationData: &authd.IARequest_AuthenticationData{},
				}
				iaResp, err := client.IsAuthenticated(context.Background(), iaReq)
				secondCall = fmt.Sprintf("SECOND CALL:\n\taccess: %s\n\tmsg: %s\n\terr: %v\n",
					iaResp.GetAccess(),
					iaResp.GetMsg(),
					err,
				)
			}
			<-done

			got := firstCall + secondCall
			got = permissionstests.IdempotentPermissionError(got)
			want := testutils.LoadWithUpdateFromGolden(t, got, testutils.WithGoldenPath(filepath.Join(testutils.GoldenPath(t), "IsAuthenticated")))
			require.Equal(t, want, got, "IsAuthenticated should return the expected combined data, but did not")

			// Check that cache has been updated too.
			gotDB, err := cachetests.DumpToYaml(usertests.GetManagerCache(m))
			require.NoError(t, err, "Setup: failed to dump database for comparing")
			wantDB := testutils.LoadWithUpdateFromGolden(t, gotDB, testutils.WithGoldenPath(filepath.Join(testutils.GoldenPath(t), "cache.db")))
			require.Equal(t, wantDB, gotDB, "IsAuthenticated should update the cache database as expected")

			grouptests.RequireGPasswdOutput(t, destCmdsFile, filepath.Join(testutils.GoldenPath(t), "gpasswd.output"))
		})
	}
}

func TestSetDefaultBrokerForUser(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username           string
		brokerID           string
		currentUserNotRoot bool

		wantErr bool
	}{
		"Set default broker for existing user with no broker":   {username: "usersetbroker"},
		"Update default broker for existing user with a broker": {username: "userupdatebroker"},
		"Setting local broker as default should not save on DB": {username: "userlocalbroker", brokerID: brokers.LocalBrokerName},

		"Error when not root":              {username: "usersetbroker", currentUserNotRoot: true, wantErr: true},
		"Error when username is empty":     {wantErr: true},
		"Error when user does not exist ":  {username: "doesnotexist", wantErr: true},
		"Error when broker does not exist": {username: "userwithbroker", brokerID: "does not exist", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			f, err := os.Open(filepath.Join(testutils.TestFamilyPath(t), "set-default-broker.db"))
			require.NoError(t, err, "Setup: could not open fixture database file")
			defer f.Close()
			err = cachetests.DbfromYAML(f, cacheDir)
			require.NoError(t, err, "Setup: could not prepare cache database file")

			expiration, err := time.Parse(time.DateOnly, "2004-01-01")
			require.NoError(t, err, "Setup: could not parse time for testing")

			m, err := users.NewManager(cacheDir, users.WithUserExpirationDate(expiration))
			require.NoError(t, err, "Setup: could not create user manager")
			t.Cleanup(func() { _ = m.Stop() })
			pm := newPermissionManager(t, tc.currentUserNotRoot)
			client := newPamClient(t, m, &pm)

			if tc.brokerID == "" {
				tc.brokerID = mockBrokerGeneratedID
			}

			sdbfuReq := &authd.SDBFURequest{
				BrokerId: tc.brokerID,
				Username: tc.username,
			}
			_, err = client.SetDefaultBrokerForUser(context.Background(), sdbfuReq)
			if tc.wantErr {
				require.Error(t, err, "SetDefaultBrokerForUser should return an error, but did not")
				return
			}
			require.NoError(t, err, "SetDefaultBrokerForUser should not return an error, but did")

			gpbResp, err := client.GetPreviousBroker(context.Background(), &authd.GPBRequest{Username: tc.username})
			require.NoError(t, err, "GetPreviousBroker should not return an error")
			require.Equal(t, tc.brokerID, gpbResp.GetPreviousBroker(), "SetDefaultBrokerForUser should set the default broker as expected")

			// Check that cache has been updated too.
			gotDB, err := cachetests.DumpToYaml(usertests.GetManagerCache(m))
			require.NoError(t, err, "Setup: failed to dump database for comparing")
			wantDB := testutils.LoadWithUpdateFromGolden(t, gotDB, testutils.WithGoldenPath(filepath.Join(testutils.GoldenPath(t), "cache.db")))
			require.Equal(t, wantDB, gotDB, "SetDefaultBrokerForUser should update the cache database as expected")
		})
	}
}

func TestEndSession(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sessionID string

		username           string
		currentUserNotRoot bool

		wantErr bool
	}{
		"Successfully end session": {username: "success"},

		"Error when not root":             {username: "success", currentUserNotRoot: true, wantErr: true},
		"Error when sessionID is empty":   {sessionID: "-", wantErr: true},
		"Error when sessionID is invalid": {sessionID: "invalid-session", wantErr: true},
		"Error when ending session":       {username: "ES_error", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pm := newPermissionManager(t, false) // Allow starting the session (current user considered root)
			client := newPamClient(t, nil, &pm)

			switch tc.sessionID {
			case "invalid-session":
			case "-":
				tc.sessionID = ""
			default:
				id := startSession(t, client, tc.username)
				if tc.sessionID == "" {
					tc.sessionID = id
				}
			}

			// Now, set tests permissions for this use case
			permissionstests.SetCurrentRootAsRoot(&pm, !tc.currentUserNotRoot)

			esReq := &authd.ESRequest{
				SessionId: tc.sessionID,
			}
			_, err := client.EndSession(context.Background(), esReq)
			if tc.wantErr {
				require.Error(t, err, "EndSession should return an error, but did not")
				return
			}
			require.NoError(t, err, "EndSession should not return an error, but did")
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	grouptests.Mockgpasswd(t)
}

// initBrokers starts dbus mock brokers on the system bus. It returns its config path.
func initBrokers() (brokerConfigPath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "authd-internal-pam-tests-")
	if err != nil {
		return "", nil, err
	}

	brokersConfPath := filepath.Join(tmpDir, "etc", "authd", "broker.d")
	if err = os.MkdirAll(brokersConfPath, 0750); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}
	_, brokerCleanup, err := testutils.StartBusBrokerMock(brokersConfPath, "BrokerMock")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}

	return brokersConfPath, func() {
		brokerCleanup()
		_ = os.RemoveAll(tmpDir)
	}, nil
}

// newPAMClient returns a new GRPC PAM client for tests connected to the global brokerManager with the given cache and
// permissionmanager.
// If the one passed is nil, this function will create the cache and close it upon test teardown.
func newPamClient(t *testing.T, m *users.Manager, pm *permissions.Manager) (client authd.PAMClient) {
	t.Helper()

	// socket path is limited in length.
	tmpDir, err := os.MkdirTemp("", "authd-socket-dir")
	require.NoError(t, err, "Setup: could not setup temporary socket dir path")
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	socketPath := filepath.Join(tmpDir, "authd.sock")

	lis, err := net.Listen("unix", socketPath)
	require.NoError(t, err, "Setup: could not create unix socket")

	if m == nil {
		m, err = users.NewManager(t.TempDir())
		require.NoError(t, err, "Setup: could not create user manager")
		t.Cleanup(func() { _ = m.Stop() })
	}

	service := pam.NewService(context.Background(), m, brokerManager, pm)

	grpcServer := grpc.NewServer(permissions.WithUnixPeerCreds(), grpc.UnaryInterceptor(enableCheckGlobalAccess(service)))
	authd.RegisterPAMServer(grpcServer, service)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		<-done
	})

	conn, err := grpc.Dial("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "Setup: Could not connect to GRPC server")
	t.Cleanup(func() { _ = conn.Close() }) // We don't care about the error on cleanup

	return authd.NewPAMClient(conn)
}

// newPermissionManager factors out permission manager creation for tests.
// this permission manager can then be tweaked for mimicking currentUser considered as root not.
func newPermissionManager(t *testing.T, currentUserNotRoot bool) permissions.Manager {
	t.Helper()

	var opts = []permissions.Option{}
	if !currentUserNotRoot {
		opts = append(opts, permissionstests.WithCurrentUserAsRoot())
	}
	return permissions.New(opts...)
}

// enableCheckGlobalAccess returns the middleware hooking up in CheckGlobalAccess for the given service.
func enableCheckGlobalAccess(s pam.Service) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := s.CheckGlobalAccess(ctx, info.FullMethod); err != nil {
			return nil, err
		}

		return handler(ctx, req)
	}
}

// getMockBrokerGeneratedID returns the generated ID for the mock broker.
func getMockBrokerGeneratedID(brokerManager *brokers.Manager) (string, error) {
	for _, b := range brokerManager.AvailableBrokers() {
		if b.Name != "BrokerMock" {
			continue
		}
		return b.ID, nil
	}
	return "", errors.New("Setup: could not find generated broker mock ID in the broker manager list")
}

// startSession is a helper that starts a session on the mock broker.
func startSession(t *testing.T, client authd.PAMClient, username string) string {
	t.Helper()

	// Prefixes the username to avoid concurrency issues.
	username = t.Name() + testutils.IDSeparator + username

	sbResp, err := client.SelectBroker(context.Background(), &authd.SBRequest{
		BrokerId: mockBrokerGeneratedID,
		Username: username,
		Mode:     authd.SessionMode_AUTH,
	})
	require.NoError(t, err, "Setup: failed to create session for tests")
	return sbResp.GetSessionId()
}

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	// Start system bus mock.
	busCleanup, err := testutils.StartSystemBusMock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer busCleanup()

	// Start brokers mock over dbus.
	brokersConfPath, cleanup, err := initBrokers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	// Get manager shared across grpc services.
	brokerManager, err = brokers.NewManager(context.Background(), brokersConfPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	mockBrokerGeneratedID, err = getMockBrokerGeneratedID(brokerManager)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	m.Run()
}
