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

			configDir := filepath.Join(brokerCfgs, "valid_brokers")
			if tc.wantErr {
				configDir = filepath.Join(brokerCfgs, "invalid_brokers")
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
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b := newBrokerForTests(t, "")

			if tc.supportedUILayouts == nil {
				tc.supportedUILayouts = []string{"required-entry"}
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

			got := "MODES:\n" + string(modesStr) + "\n\nVALIDATORS:\n" + b.LayoutValidatorsString(tc.sessionID)
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
		"Successfully select mode with required value":         {sessionID: "SAM_success_required_entry"},
		"Successfully select mode with optional value":         {sessionID: "SAM_success_optional_entry", supportedUILayouts: []string{"optional-entry"}},
		"Successfully select mode with missing optional value": {sessionID: "SAM_missing_optional_entry", supportedUILayouts: []string{"optional-entry"}},

		// broker errors
		"Error when selecting invalid auth mode": {sessionID: "SAM_error", wantErr: true},

		/* Layout errors */
		"Error when returns no layout":                          {sessionID: "SAM_no_layout", wantErr: true},
		"Error when returns empty layout":                       {sessionID: "SAM_empty_layout", wantErr: true},
		"Error when returns layout with no type":                {sessionID: "SAM_no_layout_type", wantErr: true},
		"Error when returns layout with invalid type":           {sessionID: "SAM_invalid_layout_type", wantErr: true},
		"Error when returns layout without required value":      {sessionID: "SAM_missing_required_entry", wantErr: true},
		"Error when returns layout with invalid required value": {sessionID: "SAM_invalid_required_entry", wantErr: true},
		"Error when returns layout with invalid optional value": {sessionID: "SAM_invalid_optional_entry", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b := newBrokerForTests(t, "")

			if tc.supportedUILayouts == nil {
				tc.supportedUILayouts = []string{"required-entry"}
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
		sessionID  string
		secondCall bool

		cancelFirstCall bool
	}{
		//TODO: Once validation is implemented, add cases to check if the data returned by the broker matches what is expected from the access code.

		"Successfully authorize":                             {sessionID: "success"},
		"Successfully authorize after cancelling first call": {sessionID: "IA_second_call", secondCall: true},
		"Denies authentication when broker times out":        {sessionID: "IA_timeout"},

		"Empty data gets JSON formatted": {sessionID: "IA_empty_data"},

		// broker errors
		"Error when authorizing":                                           {sessionID: "IA_error"},
		"Error when broker returns invalid access":                         {sessionID: "IA_invalid"},
		"Error when broker returns invalid data":                           {sessionID: "IA_invalid_data"},
		"Error when calling IsAuthorized a second time without cancelling": {sessionID: "IA_second_call", secondCall: true, cancelFirstCall: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b := newBrokerForTests(t, "")

			// Stores the combined output of both calls to IsAuthorized
			var firstCallReturn, secondCallReturn string

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				access, gotData, err := b.IsAuthorized(ctx, tc.sessionID, "password")
				firstCallReturn = fmt.Sprintf("FIRST CALL:\n\taccess: %s\n\tdata: %s\n\terr: %v\n", access, gotData, err)
			}()

			// Give some time for the first call to block
			time.Sleep(time.Second)

			if tc.secondCall {
				if !tc.cancelFirstCall {
					cancel()
					<-done
				}
				access, gotData, err := b.IsAuthorized(context.Background(), tc.sessionID, "password")
				secondCallReturn = fmt.Sprintf("SECOND CALL:\n\taccess: %s\n\tdata: %s\n\terr: %v\n", access, gotData, err)
			}

			<-done
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

			b := newBrokerForTests(t, "")

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
