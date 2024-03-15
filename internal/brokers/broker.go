package brokers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/decorate"
	"golang.org/x/exp/slices"
)

// LocalBrokerName is the name of the local broker.
const LocalBrokerName = "local"

const (
	// AuthGranted is the response when the authentication is granted.
	AuthGranted = "granted"
	// AuthDenied is the response when the authentication is denied.
	AuthDenied = "denied"
	// AuthCancelled is the response when the authentication is cancelled.
	AuthCancelled = "cancelled"
	// AuthRetry is the response when the authentication needs to be retried (another chance).
	AuthRetry = "retry"
	// AuthNext is the response when another MFA (including changing password) authentication is necessary.
	AuthNext = "next"
)

// AuthReplies is the list of all possible authentication replies.
var AuthReplies = []string{AuthGranted, AuthDenied, AuthCancelled, AuthRetry, AuthNext}

type brokerer interface {
	NewSession(ctx context.Context, username, lang, mode string) (sessionID, encryptionKey string, err error)
	GetAuthenticationModes(ctx context.Context, sessionID string, supportedUILayouts []map[string]string) (authenticationModes []map[string]string, err error)
	SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error)
	IsAuthenticated(ctx context.Context, sessionID, authenticationData string) (access, data string, err error)
	EndSession(ctx context.Context, sessionID string) (err error)
	CancelIsAuthenticated(ctx context.Context, sessionID string)

	UserPreCheck(ctx context.Context, username string) (err error)
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

	if name == LocalBrokerName {
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
func (b Broker) newSession(ctx context.Context, username, lang, mode string) (sessionID, encryptionKey string, err error) {
	sessionID, encryptionKey, err = b.brokerer.NewSession(ctx, username, lang, mode)
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

// IsAuthenticated calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b Broker) IsAuthenticated(ctx context.Context, sessionID, authenticationData string) (access string, data string, err error) {
	sessionID = b.parseSessionID(sessionID)

	// monitor ctx in goroutine to call cancel
	done := make(chan struct{})
	go func() {
		access, data, err = b.brokerer.IsAuthenticated(ctx, sessionID, authenticationData)
		close(done)
	}()

	select {
	case <-done:
		if err != nil {
			return "", "", err
		}
	case <-ctx.Done():
		b.cancelIsAuthenticated(ctx, sessionID)
		<-done
	}

	// Validate access authentication.
	if !slices.Contains(AuthReplies, access) {
		return "", "", fmt.Errorf("invalid access authentication key: %v", access)
	}

	if data == "" {
		data = "{}"
	}

	switch access {
	case AuthGranted:
		rawUserInfo, err := unmarshalAndGetKey(data, "userinfo")
		if err != nil {
			return "", "", err
		}

		info, err := unmarshalUserInfo(rawUserInfo)
		if err != nil {
			return "", "", err
		}

		u, err := validateUserInfoAndGenerateIDs(b.Name, info)
		if err != nil {
			return "", "", err
		}

		d, err := json.Marshal(u)
		if err != nil {
			return "", "", fmt.Errorf("can't marshal UserInfo: %v", err)
		}
		data = string(d)

	case AuthDenied, AuthRetry:
		if _, err := unmarshalAndGetKey(data, "message"); err != nil {
			return "", "", err
		}

	case AuthCancelled, AuthNext:
		if data != "{}" {
			return "", "", fmt.Errorf("access mode %q should not return any data, got: %v", access, data)
		}
	}

	return access, data, nil
}

// endSession calls the broker corresponding method, stripping broker ID prefix from sessionID.
func (b Broker) endSession(ctx context.Context, sessionID string) (err error) {
	sessionID = b.parseSessionID(sessionID)
	return b.brokerer.EndSession(ctx, sessionID)
}

// cancelIsAuthenticated calls the broker corresponding method.
// If the session does not have a pending IsAuthenticated call, this is a no-op.
//
// Even though this is a public method, it should only be interacted with through IsAuthenticated and ctx cancellation.
func (b Broker) cancelIsAuthenticated(ctx context.Context, sessionID string) {
	b.brokerer.CancelIsAuthenticated(ctx, sessionID)
}

// UserPreCheck calls the broker corresponding method.
func (b Broker) UserPreCheck(ctx context.Context, username string) (err error) {
	return b.brokerer.UserPreCheck(ctx, username)
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

	layoutValidators, exists := b.layoutValidators[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %q does not have any layout validator", sessionID)
	}

	// layoutValidator is UI Layout validator generated based on the supported layouts.
	layoutValidator, exists := layoutValidators[layout["type"]]
	if !exists {
		return nil, fmt.Errorf("no validator for UI layout type %q", layout["type"])
	}

	// Ensure that all fields provided in the layout returned by the broker are valid.
	for key := range layout {
		if key == "type" {
			continue
		}
		if _, exists := layoutValidator[key]; !exists {
			return nil, fmt.Errorf("unrecognized field %q provided for layout %q", key, layout["type"])
		}
	}
	// Ensure that all required fields were provided and that the values are valid.
	for key, validator := range layoutValidator {
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
	}
	return layout, nil
}

// parseSessionID strips broker ID prefix from sessionID.
func (b Broker) parseSessionID(sessionID string) string {
	return strings.TrimPrefix(sessionID, fmt.Sprintf("%s-", b.ID))
}

type userInfo struct {
	users.UserInfo
	UUID   string
	UGID   string
	Groups []struct {
		Name string
		UGID string
	}
}

// unmarshalUserInfo tries to unmarshal the rawMsg into a userinfo.
func unmarshalUserInfo(rawMsg json.RawMessage) (userInfo, error) {
	var u userInfo
	if err := json.Unmarshal(rawMsg, &u); err != nil {
		return userInfo{}, fmt.Errorf("message is not JSON formatted: %v", err)
	}
	return u, nil
}

// validateUserInfoAndGenerateIDs checks if the specified userinfo is valid and generates the UID and GIDs.
func validateUserInfoAndGenerateIDs(brokerName string, uInfo userInfo) (user users.UserInfo, err error) {
	defer decorate.OnError(&err, "provided userinfo is invalid")

	// Validate username
	if uInfo.Name == "" {
		return users.UserInfo{}, fmt.Errorf("empty username")
	}

	// Validate home and shell directories
	if !filepath.IsAbs(filepath.Clean(uInfo.Dir)) {
		return users.UserInfo{}, fmt.Errorf("value provided for homedir is not an absolute path: %s", uInfo.Dir)
	}
	if !filepath.IsAbs(filepath.Clean(uInfo.Shell)) {
		return users.UserInfo{}, fmt.Errorf("value provided for shell is not an absolute path: %s", uInfo.Shell)
	}

	// Validate UUID and generate UID
	if uInfo.UUID == "" {
		return users.UserInfo{}, fmt.Errorf("empty UUID")
	}
	uInfo.UID = generateID(brokerName + uInfo.UUID)

	// User must be a part of at least one group.
	if len(uInfo.Groups) == 0 {
		return users.UserInfo{}, fmt.Errorf("empty groups")
	}
	// The default group for the user is the default and it must have a UGID.
	if uInfo.Groups[0].UGID == "" {
		return users.UserInfo{}, fmt.Errorf("default group has empty UGID")
	}
	// Validate UGIDs and generate GIDs
	for _, g := range uInfo.Groups {
		if g.Name == "" {
			return users.UserInfo{}, fmt.Errorf("group has empty name")
		}
		var gid *int
		if g.UGID != "" {
			gidv := generateID(brokerName + g.UGID)
			gid = &gidv
		}
		uInfo.UserInfo.Groups = append(uInfo.UserInfo.Groups, users.GroupInfo{Name: g.Name, GID: gid})
	}

	return uInfo.UserInfo, nil
}

// generatedID generates an integer number based on the provided string.
func generateID(str string) int {
	var sum int
	for i, c := range str {
		// Multiplies the uint value of the rune by its index+1. Subtracts the index to add another layer of conflict prevention.
		sum += int(uint(c)*uint(i+1)) - i
	}
	return (sum % (100000 - 65537)) + 65536 // Ensures that ID is between 65536 and 100000
}

// unmarshalAndGetKey tries to unmarshal the content in data and returns the value of the requested key.
func unmarshalAndGetKey(data, key string) (json.RawMessage, error) {
	var returnedData map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &returnedData); err != nil {
		return nil, fmt.Errorf("response returned by the broker is not a valid json: %v\nBroker returned: %v", err, data)
	}

	rawMsg, ok := returnedData[key]
	if !ok {
		return nil, fmt.Errorf("missing key %q in returned message, got: %v", key, data)
	}

	return rawMsg, nil
}
