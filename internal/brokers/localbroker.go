package brokers

import (
	"context"
	"errors"
)

//nolint:unused // We still need localBroker to implement the brokerer interface, even though this type will not be interacted with by the daemon.
type localBroker struct {
}

//nolint:unused // We still need localBroker to implement the brokerer interface, even though this method should never be called on it.
func (b localBroker) NewSession(ctx context.Context, username, lang string) (sessionID, encryptionKey string, err error) {
	return "", "", errors.New("NewSession should never be called on local broker")
}

//nolint:unused // We still need localBroker to implement the brokerer interface, even though this method should never be called on it.
func (b localBroker) GetAuthenticationModes(ctx context.Context, sessionID string, supportedUILayouts []map[string]string) (authenticationModes []map[string]string, err error) {
	return nil, errors.New("GetAuthenticationModes should never be called on local broker")
}

//nolint:unused // We still need localBroker to implement the brokerer interface, even though this method should never be called on it.
func (b localBroker) SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error) {
	return nil, errors.New("SelectAuthenticationMode should never be called on local broker")
}

//nolint:unused // We still need localBroker to implement the brokerer interface, even though this method should never be called on it.
func (b localBroker) IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access, data string, err error) {
	return "", "", errors.New("IsAuthorized should never be called on local broker")
}

//nolint:unused // We still need localBroker to implement the brokerer interface, even though this method should never be called on it.
func (b localBroker) EndSession(ctx context.Context, sessionID string) (err error) {
	return errors.New("EndSession should never be called on local broker")
}

//nolint:unused // We still need localBroker to implement the brokerer interface, even though this method should never be called on it.
func (b localBroker) CancelIsAuthorized(ctx context.Context, sessionID string) {
}
