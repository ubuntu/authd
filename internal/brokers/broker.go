package brokers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/responses"
	"github.com/ubuntu/decorate"
	"golang.org/x/exp/slices"
)

const (
	localBrokerName = "local"
)

type brokerer interface {
	NewSession(ctx context.Context, username, lang string) (sessionID, encryptionKey string, err error)
	GetAuthenticationModes(ctx context.Context, sessionID string, supportedUILayouts []map[string]string) (authenticationModes []map[string]string, err error)
	SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error)
	IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access, data string, err error)
	EndSession(ctx context.Context, sessionID string) (err error)
	CancelIsAuthorized(ctx context.Context, sessionID string)
}

// Broker represents a broker object that can be used for authentication.
type Broker struct {
	ID                 string
	Name               string
	BrandIconPath      string
	layoutValidators   map[string]map[string]layoutValidator
	layoutValidatorsMu *sync.Mutex
	brokerer           brokerer
}

type layoutValidator map[string]fieldValidator

type fieldValidator struct {
	supportedValues []string
	required        bool
}

// newBroker creates a new broker object based on the provided name and config file.
func newBroker(ctx context.Context, name, configFile string, bus *dbus.Conn) (b Broker, err error) {
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
	}

	return Broker{
		ID:                 id,
		Name:               fullName,
		BrandIconPath:      brandIcon,
		brokerer:           broker,
		layoutValidators:   make(map[string]map[string]layoutValidator),
		layoutValidatorsMu: &sync.Mutex{},
	}, nil
}

// newSession calls the broker corresponding method, expanding sessionID with the broker ID prefix.
func (b Broker) newSession(ctx context.Context, username, lang string) (sessionID, encryptionKey string, err error) {
	sessionID, encryptionKey, err = b.brokerer.NewSession(ctx, username, lang)
	if err != nil {
		return "", "", err
	}

	if sessionID == "" {
		return "", "", errors.New("no session ID provided by broker")
	}

	return fmt.Sprintf("%s-%s", b.ID, sessionID), encryptionKey, nil
}

// GetAuthenticationModes calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b *Broker) GetAuthenticationModes(ctx context.Context, sessionID string, supportedUILayouts []map[string]string) (authenticationModes []map[string]string, err error) {
	sessionID = b.parseSessionID(sessionID)

	b.layoutValidatorsMu.Lock()
	b.layoutValidators[sessionID] = generateValidators(ctx, sessionID, supportedUILayouts)
	b.layoutValidatorsMu.Unlock()

	authenticationModes, err = b.brokerer.GetAuthenticationModes(ctx, sessionID, supportedUILayouts)
	if err != nil {
		return nil, err
	}

	for _, a := range authenticationModes {
		for _, key := range []string{"id", "label"} {
			if _, exists := a[key]; !exists {
				return nil, fmt.Errorf("invalid authentication mode, missing %q key: %v", key, a)
			}
		}
	}

	return authenticationModes, nil
}

// SelectAuthenticationMode calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b Broker) SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error) {
	sessionID = b.parseSessionID(sessionID)
	uiLayoutInfo, err = b.brokerer.SelectAuthenticationMode(ctx, sessionID, authenticationModeName)
	if err != nil {
		return nil, err
	}
	return b.validateUILayout(sessionID, uiLayoutInfo)
}

// IsAuthorized calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b Broker) IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access string, data string, err error) {
	sessionID = b.parseSessionID(sessionID)

	// monitor ctx in goroutine to call cancel
	done := make(chan struct{})
	go func() {
		access, data, err = b.brokerer.IsAuthorized(ctx, sessionID, authenticationData)
		close(done)
	}()

	select {
	case <-done:
		if err != nil {
			return "", "", err
		}
	case <-ctx.Done():
		b.cancelIsAuthorized(ctx, sessionID)
		<-done
	}

	// Validate access authorization.
	if !slices.Contains(responses.AuthReplies, access) {
		return "", "", fmt.Errorf("invalid access authorization key: %v", access)
	}

	// Validate json
	if data == "" {
		data = "{}"
	}
	if !json.Valid([]byte(data)) {
		return "", "", fmt.Errorf("invalid user information (not json formatted): %v", data)
	}

	return access, data, nil
}

// endSession calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b Broker) endSession(ctx context.Context, sessionID string) (err error) {
	sessionID = b.parseSessionID(sessionID)
	return b.brokerer.EndSession(ctx, sessionID)
}

// cancelIsAuthorized calls the broker corresponding method.
// If the session does not have a pending IsAuthorized call, this is a no-op.
//
// Even though this is a public method, it should only be interacted with through IsAuthorized and ctx cancellation.
func (b Broker) cancelIsAuthorized(ctx context.Context, sessionID string) {
	b.brokerer.CancelIsAuthorized(ctx, sessionID)
}

// generateValidators generates layout validators based on what is supported by the system.
//
// The layout validators are in the form:
//
//	{
//	    "LAYOUT_TYPE": {
//	        "FIELD_NAME": fieldValidator{
//	            supportedValues: []string{"SUPPORTED_VALUE_1", "SUPPORTED_VALUE_2"},
//	            required: true,
//	        }
//	    }
//	}
func generateValidators(ctx context.Context, sessionID string, supportedUILayouts []map[string]string) map[string]layoutValidator {
	validators := make(map[string]layoutValidator)
	for _, layout := range supportedUILayouts {
		if _, exists := layout["type"]; !exists {
			log.Errorf(ctx, "layout %v provided with missing type for session %s, it will be ignored", layout, sessionID)
			continue
		}

		layoutValidator := make(layoutValidator)
		for key, value := range layout {
			if key == "type" {
				continue
			}

			required, supportedValues, _ := strings.Cut(value, ":")
			validator := fieldValidator{
				supportedValues: nil,
				required:        (required == "required"),
			}
			if supportedValues != "" {
				values := strings.Split(supportedValues, ",")
				for _, value := range values {
					validator.supportedValues = append(validator.supportedValues, strings.TrimSpace(value))
				}
			}
			layoutValidator[key] = validator
		}
		validators[layout["type"]] = layoutValidator
	}
	return validators
}

// validateUILayout validates the layout fields and content according to the broker validators and returns the layout
// containing all required fields and the optional fields that were set.
//
// If the layout is not valid (missing required fields or invalid values), an error is returned instead.
func (b Broker) validateUILayout(sessionID string, layout map[string]string) (r map[string]string, err error) {
	defer decorate.OnError(&err, "could not validate UI layout")

	b.layoutValidatorsMu.Lock()
	defer b.layoutValidatorsMu.Unlock()

	layoutValidator, exists := b.layoutValidators[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %q does not have any layout validator", sessionID)
	}

	typ := layout["type"]
	layoutTypeValidator, exists := layoutValidator[typ]
	if !exists {
		return nil, fmt.Errorf("no validator for UI layout type %q", typ)
	}

	r = map[string]string{"type": typ}
	for key, validator := range layoutTypeValidator {
		value, exists := layout[key]
		if !exists || value == "" {
			if validator.required {
				return nil, fmt.Errorf("required field %q was not provided", key)
			}
			continue
		}

		if validator.supportedValues != nil && !slices.Contains(validator.supportedValues, value) {
			return nil, fmt.Errorf("field %q has invalid value %q, expected one of %s", key, value, strings.Join(validator.supportedValues, ","))
		}
		r[key] = value
	}
	return r, nil
}

// parseSessionID strips broker ID prefix from sessionID.
func (b Broker) parseSessionID(sessionID string) string {
	return strings.TrimPrefix(sessionID, fmt.Sprintf("%s-", b.ID))
}
