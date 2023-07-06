package brokers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
	"golang.org/x/exp/slices"
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
	id := fmt.Sprint(h.Sum32())

	if name == localBrokerName {
		id = name
	}

	fullName := name
	var broker brokerer
	var brandIcon string
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
		ID:            id,
		Name:          fullName,
		BrandIconPath: brandIcon,
		brokerer:      broker,
	}, nil
}

// GetAuthenticationModes calls the broker corresponding method, expanding sessionID with the broker ID prefix.
// This solves the case of 2 brokers returning the same ID.
func (b Broker) GetAuthenticationModes(ctx context.Context, username, lang string, supportedUiLayouts []map[string]string) (sessionID, encryptionKey string, authenticationModes []map[string]string, err error) {
	sessionID, encryptionKey, authenticationModes, err = b.brokerer.GetAuthenticationModes(ctx, username, lang, supportedUiLayouts)
	if err != nil {
		return "", "", nil, err
	}

	if sessionID == "" {
		return "", "", nil, errors.New("no session ID provided by broker")
	}

	for _, a := range authenticationModes {
		for _, key := range []string{"name", "label"} {
			if _, exists := a[key]; !exists {
				return "", "", nil, fmt.Errorf("invalid authentication mode, missing %q key: %v", key, a)
			}
		}
	}

	return fmt.Sprintf("%s-%s", b.ID, sessionID), encryptionKey, authenticationModes, nil
}

// SelectAuthenticationMode calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b Broker) SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error) {
	sessionID = strings.TrimPrefix(sessionID, fmt.Sprintf("%s-", b.ID))
	uiLayoutInfo, err = b.brokerer.SelectAuthenticationMode(ctx, sessionID, authenticationModeName)
	if err != nil {
		return nil, err
	}

	return validateUILayout(uiLayoutInfo)
}

// IsAuthorized calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b Broker) IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access string, userInfo string, err error) {
	sessionID = strings.TrimPrefix(sessionID, fmt.Sprintf("%s-", b.ID))

	access, userInfo, err = b.brokerer.IsAuthorized(ctx, sessionID, authenticationData)
	if err != nil {
		return "", "", err
	}

	// Validate access authorization.
	if !slices.Contains([]string{"allowed", "denied"}, access) {
		return "", "", fmt.Errorf("invalid access authorization key: %v", access)
	}

	// Validate json
	if userInfo == "" {
		userInfo = "{}"
	}
	if !json.Valid([]byte(userInfo)) {
		return "", "", fmt.Errorf("invalid user information (not json formatted): %v", userInfo)
	}

	return access, userInfo, nil
}

// validateUILayout validates the required fields and values for a given type.
// It returns only the required and optional fields for a given type.
func validateUILayout(layout map[string]string) (r map[string]string, err error) {
	defer decorate.OnError(&err, "no valid UI layouts metadata")

	typ := layout["type"]
	label := layout["label"]
	entry := layout["entry"]
	button := layout["button"]
	wait := layout["wait"]
	content := layout["content"]

	switch typ {
	case "form":
		if label == "" {
			return nil, fmt.Errorf("'label' is required")
		}
		if !slices.Contains([]string{"chars", "digits", "chars_password", "digits_password", ""}, entry) {
			return nil, fmt.Errorf("'entry' does not match allowed values for this type: %v", entry)
		}
		if !slices.Contains([]string{"true", "false", ""}, wait) {
			return nil, fmt.Errorf("'wait' does not match allowed values for this type: %v", wait)
		}
		r["label"] = label
		r["entry"] = entry
		r["button"] = button
		r["wait"] = wait
	case "qrcode":
		if content == "" {
			return nil, fmt.Errorf("'content' is required")
		}
		if !slices.Contains([]string{"true", "false"}, wait) {
			return nil, fmt.Errorf("'wait' is required and does not match allowed values for this type: %v", wait)
		}
		r["content"] = content
		r["wait"] = wait
		r["label"] = label
		r["entry"] = entry
		r["button"] = button
	case "webview":
	default:
		return nil, fmt.Errorf("invalid layout option: type is required, got: %v", layout)
	}

	r["type"] = typ

	return r, nil
}
