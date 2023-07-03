package brokers

import (
	"context"
	"errors"
)

type localBroker struct {
}

// Those should never be called
func (b localBroker) GetAuthenticationModes(ctx context.Context, username, lang string, supportedUiLayouts []map[string]string) (sessionID, encryptionKey string, authenticationModes []map[string]string, err error) {
	return "", "", nil, errors.New("GetAuthenticationModes should never be called on local broker")
}
func (b localBroker) SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error) {
	return nil, errors.New("SelectAuthenticationMode should never be called on local broker")
}
func (b localBroker) IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access, infoUser string, err error) {
	return "", "", errors.New("IsAuthorized should never be called on local broker")
}
