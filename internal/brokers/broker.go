package brokers

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
	"gopkg.in/ini.v1"
)

type Broker struct {
	name          string
	brandIconPath string
	dbusObject    dbus.BusObject
	interfaceName string
}

func NewBroker(ctx context.Context, name, configFile string, bus *dbus.Conn) (b Broker, err error) {
	defer decorate.OnError(&err, "can't create broker from %q", configFile)

	log.Debugf(ctx, "Loading broker configuration %q", configFile)

	cfg, err := ini.Load(configFile)
	if err != nil {
		return b, fmt.Errorf("could not read ini configuration for broker %q: %v", name, err)
	}

	fullName, err := cfg.Section("").GetKey("name")
	if err != nil {
		return b, fmt.Errorf("missing field for broker %q: %v", name, err)
	}

	brandIcon, err := cfg.Section("").GetKey("brand_icon")
	if err != nil {
		return b, fmt.Errorf("missing field for broker %q: %v", name, err)
	}

	n, err := cfg.Section("dbus").GetKey("name")
	if err != nil {
		return b, fmt.Errorf("missing field for broker %q: %v", name, err)
	}

	o, err := cfg.Section("dbus").GetKey("object")
	if err != nil {
		return b, fmt.Errorf("missing field for broker %q: %v", name, err)
	}

	interfaceName, err := cfg.Section("dbus").GetKey("interface")
	if err != nil {
		return b, fmt.Errorf("missing field for broker %q: %v", name, err)
	}

	return Broker{
		name:          fullName.String(),
		brandIconPath: brandIcon.String(),
		dbusObject:    bus.Object(n.String(), dbus.ObjectPath(o.String())),
		interfaceName: interfaceName.String(),
	}, nil
}

// Info is the textual and graphical representation of a broker information.
func (b Broker) Info() (name, brandIconPath string) {
	return b.name, b.brandIconPath
}
