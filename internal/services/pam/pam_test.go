package pam_test

import (
	"context"
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
	"github.com/ubuntu/authd/internal/services/pam"
	"github.com/ubuntu/authd/internal/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	testClient        authd.PAMClient
	brokerGeneratedID string
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

func TestAvailableBrokers(t *testing.T) {
	t.Parallel()

	abResp, err := testClient.AvailableBrokers(context.Background(), &authd.Empty{})
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

	// Try to get the broker for the user before assigning it.
	gotResp, _ := testClient.GetPreviousBroker(context.Background(), &authd.GPBRequest{Username: username})
	require.Empty(t, gotResp.GetPreviousBroker(), "GetPreviousBroker should return nil when the user has no broker assigned")

	_, err := testClient.SetDefaultBrokerForUser(context.Background(), &authd.SDBFURequest{
		BrokerId: "local",
		Username: username,
	})
	require.NoError(t, err, "Setup: could not set default broker for user for tests")

	// Assert that the broker assigned to the user is correct.
	gotResp, _ = testClient.GetPreviousBroker(context.Background(), &authd.GPBRequest{Username: username})
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

			if tc.brokerID == "" {
				tc.brokerID = brokerGeneratedID
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
			sbResp, err := testClient.SelectBroker(context.Background(), sbRequest)
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
		username  string
		noSession bool

		// This is the expected return.
		wantErr bool
	}{
		"Successfully get authentication modes":          {},
		"Successfully get multiple authentication modes": {username: "GAM_multiple_modes"},

		"Error when sessionID is empty":           {sessionID: "-", wantErr: true},
		"Error when passing invalid layout":       {supportedUILayouts: []*authd.UILayout{emptyType}, wantErr: true},
		"Error when broker does not exist":        {sessionID: "no broker", noSession: true, wantErr: true},
		"Error when getting authentication modes": {username: "GAM_error", wantErr: true},
		"Error when broker returns invalid modes": {username: "GAM_invalid", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !tc.noSession {
				id := startSession(t, tc.username)
				if tc.sessionID == "" {
					tc.sessionID = id
				}
			}
			if tc.sessionID == "-" {
				tc.sessionID = ""
			}

			if tc.supportedUILayouts == nil {
				tc.supportedUILayouts = []*authd.UILayout{requiredEntry}
			}

			gamReq := &authd.GAMRequest{
				SessionId:          tc.sessionID,
				SupportedUiLayouts: tc.supportedUILayouts,
			}
			gamResp, err := testClient.GetAuthenticationModes(context.Background(), gamReq)
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
		noSession          bool

		// This is the expected return.
		wantErr bool
	}{
		"Successfully select mode with required value":         {username: "SAM_success_required_entry", supportedUILayouts: []*authd.UILayout{requiredEntry}},
		"Successfully select mode with missing optional value": {username: "SAM_missing_optional_entry", supportedUILayouts: []*authd.UILayout{optionalEntry}},

		// service errors
		"Error when broker does not exist":   {sessionID: "no broker", noSession: true, wantErr: true},
		"Error when sessionID is empty":      {sessionID: "-", wantErr: true},
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

			if !tc.noSession {
				id := startSession(t, tc.username)
				if tc.sessionID == "" {
					tc.sessionID = id
				}
			}
			if tc.sessionID == "-" {
				tc.sessionID = ""
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
				_, err := testClient.GetAuthenticationModes(context.Background(), gamReq)
				require.NoError(t, err, "Setup: failed to get authentication modes for tests")
			}

			samReq := &authd.SAMRequest{
				SessionId:            tc.sessionID,
				AuthenticationModeId: tc.authMode,
			}
			samResp, err := testClient.SelectAuthenticationMode(context.Background(), samReq)
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

func TestIsAuthorized(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		// These are the function arguments.
		sessionID string

		// These are auxiliary inputs that affect the test setup and help control the mock output.
		username        string
		noSession       bool
		secondCall      bool
		cancelFirstCall bool
	}{
		"Successfully authorize":                           {},
		"Successfully authorize if first call is canceled": {username: "IA_second_call", secondCall: true, cancelFirstCall: true},
		"Denies authentication when broker times out":      {username: "IA_timeout"},

		"Empty data gets JSON formatted": {username: "IA_empty_data"},

		// service errors
		"Error when sessionID is empty": {sessionID: "-"},
		"Error when there is no broker": {sessionID: "no broker", noSession: true},

		// broker errors
		"Error when authorizing":                            {username: "IA_error"},
		"Error when broker returns invalid access":          {username: "IA_invalid"},
		"Error when broker returns invalid data":            {username: "IA_invalid_data"},
		"Error when calling second time without cancelling": {username: "IA_second_call", secondCall: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !tc.noSession {
				id := startSession(t, tc.username)
				if tc.sessionID == "" {
					tc.sessionID = id
				}
			}
			if tc.sessionID == "-" {
				tc.sessionID = ""
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
				iaResp, err := testClient.IsAuthorized(ctx, iaReq)
				firstCall = fmt.Sprintf("FIRST CALL:\n\taccess: %s\n\tdata: %s\n\terr: %v\n",
					iaResp.GetAccess(),
					iaResp.GetData(),
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
				iaResp, err := testClient.IsAuthorized(context.Background(), iaReq)
				secondCall = fmt.Sprintf("SECOND CALL:\n\taccess: %s\n\tdata: %s\n\terr: %v\n",
					iaResp.GetAccess(),
					iaResp.GetData(),
					err,
				)
			}
			<-done

			got := firstCall + secondCall
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "IsAuthorized should return the expected combined data, but did not")
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

			wantID := brokerGeneratedID
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
			_, err := testClient.SetDefaultBrokerForUser(context.Background(), sdbfuReq)
			if tc.wantErr {
				require.Error(t, err, "SetDefaultBrokerForUser should return an error, but did not")
				return
			}
			require.NoError(t, err, "SetDefaultBrokerForUser should not return an error, but did")

			gotResp, _ := testClient.GetPreviousBroker(context.Background(), &authd.GPBRequest{Username: tc.username})
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
		username  string
		noSession bool

		// This is the expected return.
		wantErr bool
	}{
		"Successfully end session": {username: "success"},

		"Error when sessionID is empty":    {sessionID: "-", wantErr: true},
		"Error when sessionID is invalid":  {sessionID: "invalid", wantErr: true},
		"Error when broker does not exist": {sessionID: "no broker", noSession: true, wantErr: true},
		"Error when ending session":        {username: "ES_error", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !tc.noSession {
				id := startSession(t, tc.username)
				if tc.sessionID == "" {
					tc.sessionID = id
				}
			}
			if tc.sessionID == "-" {
				tc.sessionID = ""
			}

			esReq := &authd.ESRequest{
				SessionId: tc.sessionID,
			}
			_, err := testClient.EndSession(context.Background(), esReq)
			if tc.wantErr {
				require.Error(t, err, "EndSession should return an error, but did not")
				return
			}
			require.NoError(t, err, "EndSession should not return an error, but did")
		})
	}
}

func startClient() (client authd.PAMClient, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "authd-internal-pam-tests-")
	if err != nil {
		return nil, nil, err
	}

	brokerDir := filepath.Join(tmpDir, "etc", "authd", "broker.d")
	if err = os.MkdirAll(brokerDir, 0750); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, nil, err
	}
	_, brokerCleanup, err := testutils.StartBusBrokerMock(brokerDir, "BrokerMock")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, nil, err
	}

	socketPath := filepath.Join(tmpDir, "authd.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		brokerCleanup()
		return nil, nil, err
	}
	// We want everyone to be able to write to our socket and we will filter permissions
	// #nosec G302
	if err = os.Chmod(socketPath, 0666); err != nil {
		_ = os.RemoveAll(tmpDir)
		brokerCleanup()
		return nil, nil, err
	}

	brokerManager, err := brokers.NewManager(context.Background(), nil, brokers.WithRootDir(tmpDir))
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		brokerCleanup()
		return nil, nil, err
	}

	grpcServer := grpc.NewServer()
	service := pam.NewService(context.Background(), brokerManager)
	authd.RegisterPAMServer(grpcServer, service)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = grpcServer.Serve(lis)
	}()

	conn, err := grpc.Dial("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		grpcServer.Stop()
		<-done
		brokerCleanup()
		_ = os.RemoveAll(tmpDir)
		return nil, nil, err
	}

	return authd.NewPAMClient(conn), func() {
		conn.Close()
		grpcServer.Stop()
		<-done
		brokerCleanup()
		_ = os.RemoveAll(tmpDir)
	}, nil
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

	var cleanup func()
	testClient, cleanup, err = startClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		busCleanup()
		os.Exit(1)
	}
	defer cleanup()

	brokerGeneratedID = getBrokerGeneratedID(testClient, "BrokerMock")
	if brokerGeneratedID == "" {
		fmt.Fprintf(os.Stderr, "could not get generated ID for BrokerMock\n")
		cleanup()
		busCleanup()
		os.Exit(1)
	}
	m.Run()
}

// getBrokerGeneratedID returns the generated ID for the specified broker.
func getBrokerGeneratedID(client authd.PAMClient, brokerName string) string {
	r, _ := client.AvailableBrokers(context.Background(), &authd.Empty{})
	for _, b := range r.GetBrokersInfos() {
		if b.GetName() != brokerName {
			continue
		}
		return b.Id
	}
	return ""
}

// startSession is a helper that starts a session on the specified broker.
func startSession(t *testing.T, username string) string {
	t.Helper()

	// Prefixes the username to avoid concurrency issues.
	username = t.Name() + testutils.IDSeparator + username

	sbResp, err := testClient.SelectBroker(context.Background(), &authd.SBRequest{
		BrokerId: brokerGeneratedID,
		Username: username,
	})
	require.NoError(t, err, "Setup: failed to create session for tests")
	return sbResp.GetSessionId()
}
