package brokers

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
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
	} else {
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
