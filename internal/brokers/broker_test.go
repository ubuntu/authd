package brokers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/testutils"
)

var supportedLayouts = map[string]map[string]string{
	"required-entry": {
		"type":  "required-entry",
		"entry": "required:entry_type,other_entry_type",
	},
	"optional-entry": {
		"type":  "optional-entry",
		"entry": "optional:entry_type,other_entry_type",
	},
	"missing-type": {
		"entry": "required:missing_type",
	},
	"misconfigured-layout": {
		"type":  "misconfigured-layout",
		"entry": "required-but-misformatted",
	},
	"layout-with-spaces": {
		"type":  "layout-with-spaces",
		"entry": "required: entry_type, other_entry_type",
	},
}

func TestNewBroker(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name       string
		configFile string
		wantErr    bool
	}{
		"No config means local broker":                        {name: brokers.LocalBrokerName},
		"Successfully create broker with correct config file": {name: "broker", configFile: "valid"},

		// General config errors
		"Error when config file is invalid":     {configFile: "invalid", wantErr: true},
		"Error when config file does not exist": {configFile: "do not exist", wantErr: true},

		// Missing field errors
		"Error when config does not have name field":        {configFile: "no_name", wantErr: true},
		"Error when config does not have brand_icon field":  {configFile: "no_brand_icon", wantErr: true},
		"Error when config does not have dbus.name field":   {configFile: "no_dbus_name", wantErr: true},
		"Error when config does not have dbus.object field": {configFile: "no_dbus_object", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			conn, err := testutils.GetSystemBusConnection(t)
			require.NoError(t, err, "Setup: could not connect to system bus")

			configDir := filepath.Join(brokerConfFixtures, "valid_brokers")
			if tc.wantErr {
				configDir = filepath.Join(brokerConfFixtures, "invalid_brokers")
			}
			if tc.configFile != "" {
				tc.configFile = filepath.Join(configDir, tc.configFile)
			}

			got, err := brokers.NewBroker(context.Background(), tc.name, tc.configFile, conn)
			if tc.wantErr {
				require.Error(t, err, "NewBroker should return an error, but did not")
				return
			}
			require.NoError(t, err, "NewBroker should not return an error, but did")

			gotString := fmt.Sprintf("ID: %s\nName: %s\nBrand Icon: %s\n", got.ID, got.Name, got.BrandIconPath)

			wantString := testutils.LoadWithUpdateFromGolden(t, gotString)
			require.Equal(t, wantString, gotString, "NewBroker should return the expected broker, but did not")
		})
	}
}

func TestGetAuthenticationModes(t *testing.T) {
	t.Parallel()

	b := newBrokerForTests(t, "", "")

	tests := map[string]struct {
		sessionID          string
		supportedUILayouts []string

		wantErr bool
	}{
		"Get authentication modes and generate validators":                                         {sessionID: "success", supportedUILayouts: []string{"required-entry", "optional-entry"}},
		"Get authentication modes and generate validator ignoring whitespaces in supported values": {sessionID: "success", supportedUILayouts: []string{"layout-with-spaces"}},
		"Get authentication modes and ignores invalid UI layout":                                   {sessionID: "success", supportedUILayouts: []string{"required-entry", "missing-type"}},
		"Get multiple authentication modes and generate validators":                                {sessionID: "GAM_multiple_modes", supportedUILayouts: []string{"required-entry", "optional-entry"}},

		"Does not error out when no authentication modes are returned": {sessionID: "GAM_empty"},

		// broker errors
		"Error when getting authentication modes": {sessionID: "GAM_error", wantErr: true},
		"Error when broker returns invalid modes": {sessionID: "GAM_invalid", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.supportedUILayouts == nil {
				tc.supportedUILayouts = []string{"required-entry"}
			}

			var supportedUILayouts []map[string]string
			for _, layout := range tc.supportedUILayouts {
				supportedUILayouts = append(supportedUILayouts, supportedLayouts[layout])
			}

			gotModes, err := b.GetAuthenticationModes(context.Background(), prefixID(t, tc.sessionID), supportedUILayouts)
			if tc.wantErr {
				require.Error(t, err, "GetAuthenticationModes should return an error, but did not")
				return
			}
			require.NoError(t, err, "GetAuthenticationModes should not return an error, but did")

			modesStr, err := json.Marshal(gotModes)
			require.NoError(t, err, "Post: error when marshaling result")

			got := "MODES:\n" + string(modesStr) + "\n\nVALIDATORS:\n" + b.LayoutValidatorsString(prefixID(t, tc.sessionID))
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "GetAuthenticationModes should return the expected modes, but did not")
		})
	}
}

func TestSelectAuthenticationMode(t *testing.T) {
	t.Parallel()

	b := newBrokerForTests(t, "", "")

	tests := map[string]struct {
		sessionID          string
		supportedUILayouts []string

		wantErr bool
	}{
		"Successfully select mode with required value":         {sessionID: "SAM_success_required_entry"},
		"Successfully select mode with optional value":         {sessionID: "SAM_success_optional_entry", supportedUILayouts: []string{"optional-entry"}},
		"Successfully select mode with missing optional value": {sessionID: "SAM_missing_optional_entry", supportedUILayouts: []string{"optional-entry"}},

		// broker errors
		"Error when selecting invalid auth mode":              {sessionID: "SAM_error", wantErr: true},
		"Error when no validators were generated for session": {sessionID: "no-validators", wantErr: true},

		/* Layout errors */
		"Error when returns no layout":                          {sessionID: "SAM_no_layout", wantErr: true},
		"Error when returns empty layout":                       {sessionID: "SAM_empty_layout", wantErr: true},
		"Error when returns layout with no type":                {sessionID: "SAM_no_layout_type", wantErr: true},
		"Error when returns layout with invalid type":           {sessionID: "SAM_invalid_layout_type", wantErr: true},
		"Error when returns layout without required value":      {sessionID: "SAM_missing_required_entry", wantErr: true},
		"Error when returns layout with unknown field":          {sessionID: "SAM_unknown_field", wantErr: true},
		"Error when returns layout with invalid required value": {sessionID: "SAM_invalid_required_entry", wantErr: true},
		"Error when returns layout with invalid optional value": {sessionID: "SAM_invalid_optional_entry", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.supportedUILayouts == nil {
				tc.supportedUILayouts = []string{"required-entry"}
			}

			var supportedUILayouts []map[string]string
			for _, layout := range tc.supportedUILayouts {
				supportedUILayouts = append(supportedUILayouts, supportedLayouts[layout])
			}

			if tc.sessionID != "no-validators" {
				// This is normally done in the broker's GetAuthenticationModes method, but we need to do it here to test the SelectAuthenticationMode method.
				brokers.GenerateLayoutValidators(&b, prefixID(t, tc.sessionID), supportedUILayouts)
			}

			gotUI, err := b.SelectAuthenticationMode(context.Background(), prefixID(t, tc.sessionID), "mode1")
			if tc.wantErr {
				require.Error(t, err, "SelectAuthenticationMode should return an error, but did not")
				return
			}
			require.NoError(t, err, "SelectAuthenticationMode should not return an error, but did")

			wantUI := testutils.LoadWithUpdateFromGoldenYAML(t, gotUI)
			require.Equal(t, wantUI, gotUI, "SelectAuthenticationMode should return the expected mode UI, but did not")
		})
	}
}

func TestIsAuthenticated(t *testing.T) {
	t.Parallel()

	b := newBrokerForTests(t, "", "")

	tests := map[string]struct {
		sessionID  string
		secondCall bool

		cancelFirstCall bool
	}{
		"Successfully authenticate":                                        {sessionID: "success"},
		"Successfully authenticate after cancelling first call":            {sessionID: "IA_second_call", secondCall: true},
		"Denies authentication when broker times out":                      {sessionID: "IA_timeout"},
		"Adds default groups even if broker did not set them":              {sessionID: "IA_info_empty_groups"},
		"No error when auth.Next and no data":                              {sessionID: "IA_next"},
		"No error when broker returns userinfo with empty gecos":           {sessionID: "IA_info_empty_gecos"},
		"No error when broker returns userinfo with group with empty UGID": {sessionID: "IA_info_empty_ugid"},

		// broker errors
		"Error when authenticating":                                           {sessionID: "IA_error"},
		"Error on empty data even if granted":                                 {sessionID: "IA_empty_data"},
		"Error when broker returns invalid data":                              {sessionID: "IA_invalid_data"},
		"Error when broker returns invalid access":                            {sessionID: "IA_invalid_access"},
		"Error when broker returns invalid userinfo":                          {sessionID: "IA_invalid_userinfo"},
		"Error when broker returns userinfo with empty username":              {sessionID: "IA_info_empty_user_name"},
		"Error when broker returns userinfo with empty group name":            {sessionID: "IA_info_empty_group_name"},
		"Error when broker returns userinfo with empty UUID":                  {sessionID: "IA_info_empty_uuid"},
		"Error when broker returns userinfo with invalid homedir":             {sessionID: "IA_info_invalid_home"},
		"Error when broker returns userinfo with invalid shell":               {sessionID: "IA_info_invalid_shell"},
		"Error when broker returns data on auth.Next":                         {sessionID: "IA_next_with_data"},
		"Error when broker returns data on auth.Cancelled":                    {sessionID: "IA_cancelled_with_data"},
		"Error when broker returns no data on auth.Denied":                    {sessionID: "IA_denied_without_data"},
		"Error when broker returns no data on auth.Retry":                     {sessionID: "IA_retry_without_data"},
		"Error when calling IsAuthenticated a second time without cancelling": {sessionID: "IA_second_call", secondCall: true, cancelFirstCall: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Stores the combined output of both calls to IsAuthenticated
			var firstCallReturn, secondCallReturn string

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				access, gotData, err := b.IsAuthenticated(ctx, prefixID(t, tc.sessionID), "password")
				firstCallReturn = fmt.Sprintf("FIRST CALL:\n\taccess: %s\n\tdata: %s\n\terr: %v\n", access, gotData, err)
			}()

			// Give some time for the first call to block
			time.Sleep(time.Second)

			if tc.secondCall {
				if !tc.cancelFirstCall {
					cancel()
					<-done
				}
				access, gotData, err := b.IsAuthenticated(context.Background(), prefixID(t, tc.sessionID), "password")
				secondCallReturn = fmt.Sprintf("SECOND CALL:\n\taccess: %s\n\tdata: %s\n\terr: %v\n", access, gotData, err)
			}

			<-done
			gotStr := firstCallReturn + secondCallReturn
			want := testutils.LoadWithUpdateFromGolden(t, gotStr)
			require.Equal(t, want, gotStr, "IsAuthenticated should return the expected combined data, but did not")
		})
	}
}

func TestCancelIsAuthenticated(t *testing.T) {
	t.Parallel()

	b := newBrokerForTests(t, "", "")

	tests := map[string]struct {
		sessionID string

		wantAnswer string
	}{
		"Successfully cancels IsAuthenticated": {sessionID: "IA_wait", wantAnswer: brokers.AuthCancelled},
		"Call returns denied if not cancelled": {sessionID: "IA_timeout", wantAnswer: brokers.AuthDenied},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var access string
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				access, _, _ = b.IsAuthenticated(ctx, prefixID(t, tc.sessionID), "password")
				close(done)
			}()
			defer cancel()

			if tc.sessionID == "IA_wait" {
				// Give some time for the IsAuthenticated routine to start.
				time.Sleep(time.Second)
				cancel()
			}
			<-done
			require.Equal(t, tc.wantAnswer, access, "IsAuthenticated should return the expected access, but did not")
		})
	}
}

func TestUserPreCheck(t *testing.T) {
	t.Parallel()

	b := newBrokerForTests(t, "", "")

	tests := map[string]struct {
		username string

		wantErr bool
	}{
		"Successfully pre-check user": {username: "user-pre-check"},

		"Error if user is not available": {username: "unexistent", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := b.UserPreCheck(context.Background(), tc.username)
			if tc.wantErr {
				require.Error(t, err, "UserPreCheck should return an error, but did not")
				return
			}
			require.NoError(t, err, "UserPreCheck should not return an error, but did")
		})
	}
}

func newBrokerForTests(t *testing.T, cfgDir, brokerName string) (b brokers.Broker) {
	t.Helper()

	if cfgDir == "" {
		cfgDir = t.TempDir()
	}
	if brokerName == "" {
		brokerName = strings.ReplaceAll(t.Name(), "/", "_")
	}

	cfgPath, cleanup, err := testutils.StartBusBrokerMock(cfgDir, brokerName)
	require.NoError(t, err, "Setup: could not start bus broker mock")
	t.Cleanup(cleanup)

	conn, err := testutils.GetSystemBusConnection(t)
	require.NoError(t, err, "Setup: could not connect to system bus")
	t.Cleanup(func() { require.NoError(t, conn.Close(), "Teardown: Failed to close the connection") })

	b, err = brokers.NewBroker(context.Background(), brokerName, cfgPath, conn)
	require.NoError(t, err, "Setup: could not create broker")

	return b
}

// prefixID is a helper function that prefixes the given ID with the test name to avoid conflicts.
func prefixID(t *testing.T, id string) string {
	t.Helper()
	return t.Name() + testutils.IDSeparator + id
}
