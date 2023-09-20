package pam_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/cache"
	cachetests "github.com/ubuntu/authd/internal/cache/tests"
	"github.com/ubuntu/authd/internal/services/pam"
	"github.com/ubuntu/authd/internal/testutils"
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

	c, err := cache.New(t.TempDir())
	require.NoError(t, err, "Setup: could not create cache")

	service := pam.NewService(context.Background(), c, brokerManager)

	brokers, err := service.AvailableBrokers(context.Background(), &authd.Empty{})
	require.NoError(t, err, "can’t create the service directly")
	require.NotEmpty(t, brokers.BrokersInfos, "Service is created and can query the broker manager")
}

func TestAvailableBrokers(t *testing.T) {
	t.Parallel()

	client := newPamClient(t, nil)

	abResp, err := client.AvailableBrokers(context.Background(), &authd.Empty{})
	require.NoError(t, err, "AvailableBrokers should not return an error, but did")

	got := abResp.GetBrokersInfos()
	for _, broker := range got {
		broker.Id = broker.Name + "_ID"
	}
	want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
	require.Equal(t, want, got, "AvailableBrokers returned unexpected brokers")
}

func TestGetPreviousBroker(t *testing.T) {
	t.Parallel()

	username := t.Name()

	client := newPamClient(t, nil)

	// Try to get the broker for the user before assigning it.
	gotResp, _ := client.GetPreviousBroker(context.Background(), &authd.GPBRequest{Username: username})
	require.Empty(t, gotResp.GetPreviousBroker(), "GetPreviousBroker should return nil when the user has no broker assigned")

	_, err := client.SetDefaultBrokerForUser(context.Background(), &authd.SDBFURequest{
		BrokerId: "local",
		Username: username,
	})
	require.NoError(t, err, "Setup: could not set default broker for user for tests")

	// Assert that the broker assigned to the user is correct.
	gotResp, _ = client.GetPreviousBroker(context.Background(), &authd.GPBRequest{Username: username})
	require.Equal(t, "local", gotResp.GetPreviousBroker(), "GetPreviousBroker did not return the correct broker")
}

func TestSelectBroker(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		// These are the function arguments.
		brokerID string
		username string

		// This is the expected return.
		wantErr bool
	}{
		"Successfully select a broker and creates the session": {username: "success"},

		"Error when username is empty":                    {wantErr: true},
		"Error when brokerID is empty":                    {username: "empty broker", brokerID: "-", wantErr: true},
		"Error when broker does not exist":                {username: "no broker", brokerID: "does not exist", wantErr: true},
		"Error when broker does not provide a session ID": {username: "NS_no_id", wantErr: true},
		"Error when starting the session":                 {username: "NS_error", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newPamClient(t, nil)

			if tc.brokerID == "" {
				tc.brokerID = mockBrokerGeneratedID
			} else if tc.brokerID == "-" {
				tc.brokerID = ""
			}

			if tc.username != "" {
				tc.username = t.Name() + testutils.IDSeparator + tc.username
			}

			sbRequest := &authd.SBRequest{
				BrokerId: tc.brokerID,
				Username: tc.username,
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
		// These are the function arguments.
		sessionID          string
		supportedUILayouts []*authd.UILayout

		// These are auxiliary inputs that affect the test setup and help control the mock output.
		username string

		// This is the expected return.
		wantErr bool
	}{
		"Successfully get authentication modes":          {},
		"Successfully get multiple authentication modes": {username: "GAM_multiple_modes"},

		"Error when sessionID is empty":           {sessionID: "-", wantErr: true},
		"Error when passing invalid layout":       {supportedUILayouts: []*authd.UILayout{emptyType}, wantErr: true},
		"Error when sessionID is invalid":         {sessionID: "invalid-session", wantErr: true},
		"Error when getting authentication modes": {username: "GAM_error", wantErr: true},
		"Error when broker returns invalid modes": {username: "GAM_invalid", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newPamClient(t, nil)

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
		// These are the function arguments.
		sessionID string
		authMode  string

		// These are auxiliary inputs that affect the test setup and help control the mock output.
		username           string
		supportedUILayouts []*authd.UILayout
		noValidators       bool

		// This is the expected return.
		wantErr bool
	}{
		"Successfully select mode with required value":         {username: "SAM_success_required_entry", supportedUILayouts: []*authd.UILayout{requiredEntry}},
		"Successfully select mode with missing optional value": {username: "SAM_missing_optional_entry", supportedUILayouts: []*authd.UILayout{optionalEntry}},

		// service errors
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
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newPamClient(t, nil)

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

			if tc.authMode == "" {
				tc.authMode = "some mode"
			} else if tc.authMode == "-" {
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
	t.Parallel()

	tests := map[string]struct {
		// These are the function arguments.
		sessionID string

		// These are auxiliary inputs that affect the test setup and help control the mock output.
		username        string
		secondCall      bool
		cancelFirstCall bool
	}{
		"Successfully authenticate":                           {username: "success"},
		"Successfully authenticate if first call is canceled": {username: "IA_second_call", secondCall: true, cancelFirstCall: true},
		"Denies authentication when broker times out":         {username: "IA_timeout"},

		// service errors
		"Error when sessionID is empty": {sessionID: "-"},
		"Error when there is no broker": {sessionID: "invalid-session"},

		// broker errors
		"Error when authenticating":                         {username: "IA_error"},
		"Error on empty data even if granted":               {username: "IA_empty_data"},
		"Error when broker returns invalid access":          {username: "IA_invalid"},
		"Error when broker returns invalid data":            {username: "IA_invalid_data"},
		"Error when calling second time without cancelling": {username: "IA_second_call", secondCall: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, err := cache.New(t.TempDir())
			require.NoError(t, err, "Setup: could not create cache")
			t.Cleanup(func() { _ = c.Close() })
			client := newPamClient(t, c)

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

			var firstCall, secondCall string
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				iaReq := &authd.IARequest{
					SessionId:          tc.sessionID,
					AuthenticationData: "some data",
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
				time.Sleep(time.Millisecond)
				<-done
			}

			if tc.secondCall {
				iaReq := &authd.IARequest{
					SessionId:          tc.sessionID,
					AuthenticationData: "some data",
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
			want := testutils.LoadWithUpdateFromGolden(t, got, testutils.WithGoldenPath(filepath.Join(testutils.GoldenPath(t), "IsAuthenticated")))
			require.Equal(t, want, got, "IsAuthenticated should return the expected combined data, but did not")

			// Check that cache has been updated too.
			gotDB, err := cachetests.DumpToYaml(c)
			require.NoError(t, err, "Setup: dump database for comparing")
			wantDB := testutils.LoadWithUpdateFromGolden(t, gotDB, testutils.WithGoldenPath(filepath.Join(testutils.GoldenPath(t), "cache.db")))
			require.Equal(t, wantDB, gotDB, "IsAuthenticated should udpate the cache database as expected")
		})
	}
}

func TestSetDefaultBrokerForUser(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		// These are the function arguments.
		username string

		// These are auxiliary inputs that affect the test setup and help control the mock output.
		noBroker bool

		// This is the expected return.
		wantErr bool
	}{
		"Set default broker for existing user": {username: "success"},

		"Error when username is empty":     {wantErr: true},
		"Error when broker does not exist": {username: "no broker", noBroker: true, wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newPamClient(t, nil)

			wantID := mockBrokerGeneratedID
			if tc.noBroker {
				wantID = "does not exist"
			}
			if tc.username != "" {
				tc.username = t.Name() + tc.username
			}

			sdbfuReq := &authd.SDBFURequest{
				BrokerId: wantID,
				Username: tc.username,
			}
			_, err := client.SetDefaultBrokerForUser(context.Background(), sdbfuReq)
			if tc.wantErr {
				require.Error(t, err, "SetDefaultBrokerForUser should return an error, but did not")
				return
			}
			require.NoError(t, err, "SetDefaultBrokerForUser should not return an error, but did")

			gotResp, _ := client.GetPreviousBroker(context.Background(), &authd.GPBRequest{Username: tc.username})
			require.Equal(t, wantID, gotResp.GetPreviousBroker(), "SetDefaultBrokerForUser did not set the correct broker for the user")
		})
	}
}

func TestEndSession(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		// These are the function arguments.
		sessionID string

		// These are auxiliary inputs that affect the test setup and help control the mock output.
		username string

		// This is the expected return.
		wantErr bool
	}{
		"Successfully end session": {username: "success"},

		"Error when sessionID is empty":   {sessionID: "-", wantErr: true},
		"Error when sessionID is invalid": {sessionID: "invalid-session", wantErr: true},
		"Error when ending session":       {username: "ES_error", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newPamClient(t, nil)

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

// initBrokers starts dbus mock brokers on the system bus. It returns its config path.
func initBrokers() (brokerConfigPath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "authd-internal-pam-tests-")
	if err != nil {
		return "", nil, err
	}

	brokerDir := filepath.Join(tmpDir, "etc", "authd", "broker.d")
	if err = os.MkdirAll(brokerDir, 0750); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}
	_, brokerCleanup, err := testutils.StartBusBrokerMock(brokerDir, "BrokerMock")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}

	return tmpDir, func() {
		brokerCleanup()
		_ = os.RemoveAll(tmpDir)
	}, nil
}

// newPAMClient returns a new GRPC PAM client for tests connected to the global brokerManager with the given cache.
// If the one passed is nil, this function will create the cache and close it upon test teardown.
func newPamClient(t *testing.T, c *cache.Cache) (client authd.PAMClient) {
	t.Helper()

	// socket path is limited in length.
	tmpDir, err := os.MkdirTemp("", "authd-socket-dir")
	require.NoError(t, err, "Setup: could not setup temporary socket dir path")
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	socketPath := filepath.Join(tmpDir, "authd.sock")

	lis, err := net.Listen("unix", socketPath)
	require.NoError(t, err, "Setup: could not create unix socket")

	if c == nil {
		c, err = cache.New(t.TempDir())
		require.NoError(t, err, "Setup: could not create cache")
		t.Cleanup(func() { _ = c.Close() })
	}

	service := pam.NewService(context.Background(), c, brokerManager)

	grpcServer := grpc.NewServer()
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
	})
	require.NoError(t, err, "Setup: failed to create session for tests")
	return sbResp.GetSessionId()
}

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()
	flag.Parse()

	// Start system bus mock.
	busCleanup, err := testutils.StartSystemBusMock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer busCleanup()

	// Start brokers mock over dbus.
	brokersConfigPath, cleanup, err := initBrokers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	// Get manager shared across grpc services.
	brokerManager, err = brokers.NewManager(context.Background(), nil, brokers.WithRootDir(brokersConfigPath))
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
