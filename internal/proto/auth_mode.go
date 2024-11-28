package grpc

import (
	"encoding/json"

	"github.com/ubuntu/authd/api/types"
)

// AuthMode represent an authentication mode in authd protocol.
type AuthMode = GAMResponse_AuthenticationMode

func NewAuthMode(authMode types.AuthMode) *AuthMode {
	authModeJSON, err := json.Marshal(authMode)
	if err != nil {
		return nil
	}

	var authModeProto AuthMode
	err = json.Unmarshal(authModeJSON, &authModeProto)
	if err != nil {
		return nil
	}

	return &authModeProto
}
