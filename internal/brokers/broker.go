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
	id            string
	name          string
	brandIconPath string
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
		id:            fmt.Sprint(id),
		name:          fullName,
		brandIconPath: brandIcon,
		brokerer:      broker,
	}, nil
}

// Info is the textual and graphical representation of a broker information.
func (b Broker) Info() (id, name, brandIconPath string) {
	return b.id, b.name, b.brandIconPath
}
