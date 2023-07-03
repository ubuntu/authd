package brokers

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
)

const (
	localBrokerName = "local"
)

type brokerer interface {
	GetAuthenticationModes(ctx context.Context, username, lang string, supportedUiLayouts []map[string]string) (sessionID, encryptionKey string, authenticationModes []map[string]string, err error)
	SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error)
	IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access, infoUser string, err error)
}

type Broker struct {
	ID            string
	Name          string
	BrandIconPath string
	brokerer
}

func NewBroker(ctx context.Context, name, configFile string, bus *dbus.Conn) (b Broker, err error) {
	defer decorate.OnError(&err, "can't create broker %q", name)

	h := fnv.New32a()
	h.Write([]byte(name))
	id := h.Sum32()

	var broker brokerer
	var fullName, brandIcon string
	log.Debugf(ctx, "Loading broker %q", name)
	if configFile != "" {
		broker, fullName, brandIcon, err = newDbusBroker(ctx, bus, configFile)
		if err != nil {
			return Broker{}, err
		}
	} else if name != localBrokerName {
		broker, fullName, brandIcon, err = newExampleBroker(name)
		if err != nil {
			return Broker{}, err
		}
	}

	return Broker{
		ID:            fmt.Sprint(id),
		Name:          fullName,
		BrandIconPath: brandIcon,
		brokerer:      broker,
	}, nil
}

// IsLocal returns if the current broker is the local one.
func (b Broker) IsLocal() bool {
	return b.Name == localBrokerName
}

// GetAuthenticationModes calls the broker corresponding method, expanding sessionID with the broker ID prefix.
// This solves the case of 2 brokers returning the same ID.
func (b Broker) GetAuthenticationModes(ctx context.Context, username, lang string, supportedUiLayouts []map[string]string) (sessionID, encryptionKey string, authenticationModes []map[string]string, err error) {
	sessionID, encryptionKey, authenticationModes, err = b.brokerer.GetAuthenticationModes(ctx, username, lang, supportedUiLayouts)
	if err != nil {
		return "", "", nil, err
	}

	return fmt.Sprintf("%s-%s", b.ID, sessionID), encryptionKey, authenticationModes, nil
}

// SelectAuthenticationMode calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b Broker) SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error) {
	sessionID = strings.TrimPrefix(sessionID, fmt.Sprintf("%s-", b.ID))
	return b.brokerer.SelectAuthenticationMode(ctx, sessionID, authenticationModeName)
}

// IsAuthorized calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b Broker) IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access, infoUser string, err error) {
	sessionID = strings.TrimPrefix(sessionID, fmt.Sprintf("%s-", b.ID))
	return b.brokerer.IsAuthorized(ctx, sessionID, authenticationData)
}
