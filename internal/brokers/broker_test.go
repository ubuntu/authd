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
	"github.com/ubuntu/authd/internal/responses"
	"github.com/ubuntu/authd/internal/testutils"
)

var supportedLayouts = map[string]map[string]string{
	"required-value": {
		"type":  "required-value",
		"value": "required:value_type,other_value_type",
	},
	"optional-value": {
		"type":  "optional-value",
		"value": "optional:value_type,other_value_type",
	},
	"missing-type": {
		"value": "required:missing_type",
	},
	"misconfigured-layout": {
		"type":  "misconfigured-layout",
		"value": "required-but-misformatted",
	},
	"layout-with-spaces": {
		"type":  "layout-with-spaces",
		"value": "required: value_type, other_value_type",
	},
}

func TestNewBroker(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name       string
		configFile string
		wantErr    bool
	}{
		"No config means local broker":                        {name: "local"},
		"Successfully create broker with correct config file": {name: "broker", configFile: "valid"},

		// General config errors
		"Error when config file is invalid":     {configFile: "invalid", wantErr: true},
		"Error when config file does not exist": {configFile: "do not exist", wantErr: true},

		// Missing field errors
		"Error when config does not have name field":           {configFile: "no_name", wantErr: true},
		"Error when config does not have brand_icon field":     {configFile: "no_brand_icon", wantErr: true},
		"Error when config does not have dbus.name field":      {configFile: "no_dbus_name", wantErr: true},
		"Error when config does not have dbus.object field":    {configFile: "no_dbus_object", wantErr: true},
		"Error when config does not have dbus.interface field": {configFile: "no_dbus_interface", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			conn, err := testutils.GetSystemBusConnection(t)
			require.NoError(t, err, "Setup: could not connect to system bus")

			configDir := filepath.Join(fixturesPath, "valid_brokers")
			if tc.wantErr {
				configDir = filepath.Join(fixturesPath, "invalid_brokers")
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

	tests := map[string]struct {
		sessionID          string
		supportedUILayouts []string

		wantErr bool
	}{
		"Get authentication modes and generate validators":                                         {sessionID: "success", supportedUILayouts: []string{"required-value", "optional-value"}},
		"Get authentication modes and generate validator ignoring whitespaces in supported values": {sessionID: "success", supportedUILayouts: []string{"layout-with-spaces"}},
		"Get authentication modes and ignores invalid UI layout":                                   {sessionID: "success", supportedUILayouts: []string{"required-value", "missing-type"}},
		"Get multiple authentication modes and generate validators":                                {sessionID: "GAM_multiple_modes", supportedUILayouts: []string{"required-value", "optional-value"}},

		"Does not error out when no authentication modes are returned": {sessionID: "GAM_empty"},

		// broker errors
		"Error when getting authentication modes": {sessionID: "GAM_error", wantErr: true},
		"Error when broker returns invalid modes": {sessionID: "GAM_invalid", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b, _ := newBrokerForTests(t)

			if tc.supportedUILayouts == nil {
				tc.supportedUILayouts = []string{"required-value"}
			}

			var supportedUILayouts []map[string]string
			for _, layout := range tc.supportedUILayouts {
				supportedUILayouts = append(supportedUILayouts, supportedLayouts[layout])
			}

			gotModes, err := b.GetAuthenticationModes(context.Background(), tc.sessionID, supportedUILayouts)
			if tc.wantErr {
				require.Error(t, err, "GetAuthenticationModes should return an error, but did not")
				return
			}
			require.NoError(t, err, "GetAuthenticationModes should not return an error, but did")

			modesStr, err := json.Marshal(gotModes)
			require.NoError(t, err, "Post: error when marshaling result")

			got := "MODES:\n" + string(modesStr) + "\n\nVALIDATORS:\n" + b.LayoutValidatorsString()
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "GetAuthenticationModes should return the expected modes, but did not")
		})
	}
}

func TestSelectAuthenticationMode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sessionID          string
		supportedUILayouts []string

		wantErr bool
	}{
		"Successfully select mode with required value":         {sessionID: "SAM_success_required_value"},
		"Successfully select mode with optional value":         {sessionID: "SAM_success_optional_value", supportedUILayouts: []string{"optional-value"}},
		"Successfully select mode with missing optional value": {sessionID: "SAM_missing_optional_value", supportedUILayouts: []string{"optional-value"}},

		// broker errors
		"Error when selecting invalid auth mode": {sessionID: "SAM_error", wantErr: true},

		/* Layout errors */
		"Error when returns no layout":                          {sessionID: "SAM_no_layout", wantErr: true},
		"Error when returns empty layout":                       {sessionID: "SAM_empty_layout", wantErr: true},
		"Error when returns layout with no type":                {sessionID: "SAM_no_layout_type", wantErr: true},
		"Error when returns layout with invalid type":           {sessionID: "SAM_invalid_layout_type", wantErr: true},
		"Error when returns layout without required value":      {sessionID: "SAM_missing_required_value", wantErr: true},
		"Error when returns layout with invalid required value": {sessionID: "SAM_invalid_required_value", wantErr: true},
		"Error when returns layout with invalid optional value": {sessionID: "SAM_invalid_optional_value", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b, _ := newBrokerForTests(t)

			if tc.supportedUILayouts == nil {
				tc.supportedUILayouts = []string{"required-value"}
			}

			var supportedUILayouts []map[string]string
			for _, layout := range tc.supportedUILayouts {
				supportedUILayouts = append(supportedUILayouts, supportedLayouts[layout])
			}
			// This is normally done in the broker's GetAuthenticationModes method, but we need to do it here to test the SelectAuthenticationMode method.
			brokers.GenerateLayoutValidators(&b, tc.sessionID, supportedUILayouts)

			gotUI, err := b.SelectAuthenticationMode(context.Background(), tc.sessionID, "mode1")
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

func TestIsAuthorized(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sessionID           string
		sessionIDSecondCall string

		wantAccess           string
		wantErr              bool
		wantAccessSecondCall string
		wantErrSecondCall    bool
	}{
		//TODO: Once validation is implemented, add cases to check if the data returned by the broker matches what is expected from the access code.

		"Successfully authorize":                             {sessionID: "success", wantAccess: responses.AuthAllowed},
		"Successfully authorize after cancelling first call": {sessionID: "IA_second_call", wantAccess: responses.AuthCancelled, sessionIDSecondCall: "success", wantAccessSecondCall: responses.AuthAllowed},
		"Denies authentication when broker times out":        {sessionID: "IA_timeout", wantAccess: responses.AuthDenied},

		"Empty data gets JSON formatted": {sessionID: "IA_empty_data", wantAccess: responses.AuthAllowed},

		// broker errors
		"Error when authorizing":                                           {sessionID: "IA_error", wantErr: true},
		"Error when broker returns invalid access":                         {sessionID: "IA_invalid", wantErr: true},
		"Error when broker returns invalid data":                           {sessionID: "IA_invalid_data", wantErr: true},
		"Error when calling IsAuthorized a second time without cancelling": {sessionID: "IA_second_call", wantAccess: responses.AuthAllowed, sessionIDSecondCall: "IA_second_call", wantErrSecondCall: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b, _ := newBrokerForTests(t)

			// Stores the combined output of both calls to IsAuthorized
			var firstCallReturn, secondCallReturn string

			var access string
			var gotData string
			var err error
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				access, gotData, err = b.IsAuthorized(ctx, tc.sessionID, "password")
				firstCallReturn = fmt.Sprintf("FIRST CALL:\n\taccess: %s\n\tdata: %s\n\terr: %v\n", access, gotData, err)
				if tc.wantErr {
					require.Error(t, err, "IsAuthorized should return an error, but did not")
					return
				}
				require.NoError(t, err, "IsAuthorized should not return an error, but did")
			}()

			// Give some time for the first call to block
			time.Sleep(time.Second)

			if tc.wantAccessSecondCall != "" {
				cancel()
				// Wait for the cancel to go through
				time.Sleep(time.Millisecond)
			}
			if tc.sessionIDSecondCall != "" {
				access, gotData, err := b.IsAuthorized(context.Background(), tc.sessionID, "password")
				secondCallReturn = fmt.Sprintf("SECOND CALL:\n\taccess: %s\n\tdata: %s\n\terr: %v\n", access, gotData, err)
				if tc.wantErrSecondCall {
					require.Error(t, err, "IsAuthorized second call should return an error, but did not")
				} else {
					require.NoError(t, err, "IsAuthorized second call should not return an error, but did")
				}
			}

			<-done
			if tc.wantErr {
				return
			}
			gotStr := firstCallReturn + secondCallReturn
			want := testutils.LoadWithUpdateFromGolden(t, gotStr)
			require.Equal(t, want, gotStr, "IsAuthorized should return the expected combined data, but did not")
		})
	}
}

func TestCancelIsAuthorized(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sessionID string

		wantAnswer string
	}{
		"Successfully cancels IsAuthorized":    {sessionID: "IA_wait", wantAnswer: responses.AuthCancelled},
		"Call returns denied if not cancelled": {sessionID: "IA_timeout", wantAnswer: responses.AuthDenied},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b, _ := newBrokerForTests(t)

			var access string
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				access, _, _ = b.IsAuthorized(ctx, tc.sessionID, "password")
				close(done)
			}()
			defer cancel()

			if tc.sessionID == "IA_wait" {
				// Give some time for the IsAuthorized routine to start.
				time.Sleep(time.Second)
				cancel()
			}
			<-done
			require.Equal(t, tc.wantAnswer, access, "IsAuthorized should return the expected access, but did not")
		})
	}
}

func newBrokerForTests(t *testing.T, cfgDir string) (b brokers.Broker) {
	t.Helper()

	if cfgDir == "" {
		cfgDir = t.TempDir()
	}

	cfgPath, cleanup, err := testutils.StartBusBrokerMock(cfgDir, strings.ReplaceAll(t.Name(), "/", "_"))
	require.NoError(t, err, "Setup: could not start bus broker mock")
	t.Cleanup(cleanup)

	conn, err := testutils.GetSystemBusConnection(t)
	require.NoError(t, err, "Setup: could not connect to system bus")
	t.Cleanup(func() { require.NoError(t, conn.Close(), "Teardown: Failed to close the connection") })

	b, err = brokers.NewBroker(context.Background(), strings.ReplaceAll(t.Name(), "/", "_"), cfgPath, conn)
	require.NoError(t, err, "Setup: could not create broker")

	return b
}
