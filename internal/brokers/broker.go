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
func (b Broker) IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access string, userInfo map[string]string, err error) {
	sessionID = strings.TrimPrefix(sessionID, fmt.Sprintf("%s-", b.ID))

	access, uInfo, err := b.brokerer.IsAuthorized(ctx, sessionID, authenticationData)
	if err != nil {
		return "", nil, err
	}

	// Validate access authorization.
	if !slices.Contains([]string{"access", "denied"}, access) {
		return "", nil, fmt.Errorf("invalid access authorization key: %v", access)
	}

	// Return the type to a structured data.
	if uInfo != "" {
		userInfo, err = stringToMap(uInfo)
		if err != nil {
			return "", nil, fmt.Errorf("invalid user information type: %v", uInfo)
		}
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

func stringToMap(jsonData string) (map[string]string, error) {
	var data map[string]string

	err := json.Unmarshal([]byte(jsonData), &data)
	if err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return nil, err
	}

	return data, nil
}
