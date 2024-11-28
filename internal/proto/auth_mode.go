package proto

import (
	"encoding/json"

	"github.com/ubuntu/authd/api/types"
)

// AuthMode represent an authentication mode in authd protocol.
type AuthMode = GAMResponse_AuthenticationMode

func AuthModeFromMap(m map[string]string) (*AuthMode, error) {
	authModeJSON, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	// Check if the JSON can be successfully unmarshalled into the AuthMode struct
	_, err = types.AuthModeFromJSON(authModeJSON)
	if err != nil {
		return nil, err
	}

	var authModeProto AuthMode
	err = json.Unmarshal(authModeJSON, &authModeProto)
	if err != nil {
		return nil, err
	}

	return &authModeProto, nil
}
