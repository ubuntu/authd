package gdm_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/pam/internal/gdm"
	"github.com/ubuntu/authd/pam/internal/proto"
)

// RequireEqualData ensures that data is equal by checking the marshalled values.
func RequireEqualData(t *testing.T, want any, actual any) {
	t.Helper()

	wantJSON, err := json.MarshalIndent(want, "", "  ")
	require.NoError(t, err)
	actualJSON, err := json.MarshalIndent(actual, "", "  ")
	require.NoError(t, err)

	require.Equal(t, string(wantJSON), string(actualJSON))
}

// DataToJSON is a test helper function to convert GDM data to JSON.
func DataToJSON(t *testing.T, data *gdm.Data) string {
	t.Helper()

	json, err := data.JSON()
	require.NoError(t, err)
	return string(json)
}

// SelectUserEvent generates a SelectUser event.
func SelectUserEvent(username string) *gdm.EventData {
	return &gdm.EventData{
		Type: gdm.EventType_userSelected,
		Data: &gdm.EventData_UserSelected{
			UserSelected: &gdm.Events_UserSelected{UserId: username},
		},
	}
}

// SelectBrokerEvent generates a SelectBroker event.
func SelectBrokerEvent(brokerID string) *gdm.EventData {
	return &gdm.EventData{
		Type: gdm.EventType_brokerSelected,
		Data: &gdm.EventData_BrokerSelected{
			BrokerSelected: &gdm.Events_BrokerSelected{BrokerId: brokerID},
		},
	}
}

// ChangeStageEvent generates a ChangeStage event.
func ChangeStageEvent(stage proto.Stage) *gdm.EventData {
	return &gdm.EventData{
		Type: gdm.EventType_stageChanged,
		Data: &gdm.EventData_StageChanged{
			StageChanged: &gdm.Events_StageChanged{Stage: stage},
		},
	}
}

// AuthModeSelectedEvent generates a AuthModeSelected event.
func AuthModeSelectedEvent(authModeID string) *gdm.EventData {
	return &gdm.EventData{
		Type: gdm.EventType_authModeSelected,
		Data: &gdm.EventData_AuthModeSelected{
			AuthModeSelected: &gdm.Events_AuthModeSelected{
				AuthModeId: authModeID,
			},
		},
	}
}

// IsAuthenticatedEvent generates a IsAuthenticated event.
func IsAuthenticatedEvent(item authd.IARequestAuthenticationDataItem) *gdm.EventData {
	return &gdm.EventData{
		Type: gdm.EventType_isAuthenticatedRequested,
		Data: &gdm.EventData_IsAuthenticatedRequested{
			IsAuthenticatedRequested: &gdm.Events_IsAuthenticatedRequested{
				AuthenticationData: &authd.IARequest_AuthenticationData{Item: item},
			},
		},
	}
}

// IsAuthenticatedCancelledEvent generates a IsAuthenticated event.
func IsAuthenticatedCancelledEvent() *gdm.EventData {
	return &gdm.EventData{
		Type: gdm.EventType_isAuthenticatedCancelled,
		Data: &gdm.EventData_IsAuthenticatedCancelled{},
	}
}
