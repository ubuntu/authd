// Package examplebroker implements an example broker that will be used by the authentication daemon.
package examplebroker

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ubuntu/authd/internal/brokers/responses"
	"golang.org/x/exp/slices"
)

const maxAttempts int = 5

type passwdReset int

const (
	noReset passwdReset = iota
	canReset
	mustReset
)

type sessionInfo struct {
	username        string
	selectedMode    string
	lang            string
	allModes        map[string]map[string]string
	attemptsPerMode map[string]int

	pwdChange passwdReset

	neededMfa      int
	currentMfaStep int
}

type isAuthenticatedCtx struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// Broker represents an examplebroker object.
type Broker struct {
	currentSessions        map[string]sessionInfo
	currentSessionsMu      sync.RWMutex
	userLastSelectedMode   map[string]string
	userLastSelectedModeMu sync.Mutex
	isAuthenticatedCalls   map[string]isAuthenticatedCtx
	isAuthenticatedCallsMu sync.Mutex
}

var (
	exampleUsers = map[string]string{
		"user1":            "My user",
		"user2":            "My secondary user",
		"user-mfa":         "User that needs MFA",
		"user-needs-reset": "User that needs passwd reset",
		"user-can-reset":   "User that can passwd reset",
	}
)

const (
	brokerEncryptionKey = "encryptionkey"
)

// New creates a new examplebroker object.
func New(name string) (b *Broker, fullName, brandIcon string) {
	return &Broker{
		currentSessions:        make(map[string]sessionInfo),
		currentSessionsMu:      sync.RWMutex{},
		userLastSelectedMode:   make(map[string]string),
		userLastSelectedModeMu: sync.Mutex{},
		isAuthenticatedCalls:   make(map[string]isAuthenticatedCtx),
		isAuthenticatedCallsMu: sync.Mutex{},
	}, strings.ReplaceAll(name, "_", " "), fmt.Sprintf("/usr/share/brokers/%s.png", name)
}

// NewSession creates a new session for the specified user.
func (b *Broker) NewSession(ctx context.Context, username, lang string) (sessionID, encryptionKey string, err error) {
	sessionID = uuid.New().String()
	info := sessionInfo{
		username:        username,
		lang:            lang,
		pwdChange:       noReset,
		attemptsPerMode: make(map[string]int),
	}
	switch username {
	case "user-mfa":
		info.neededMfa = 3
	case "user-needs-reset":
		info.neededMfa = 1
		info.pwdChange = mustReset
	case "user-can-reset":
		info.neededMfa = 1
		info.pwdChange = canReset
	}

	b.currentSessionsMu.Lock()
	b.currentSessions[sessionID] = info
	b.currentSessionsMu.Unlock()
	return sessionID, brokerEncryptionKey, nil
}

// GetAuthenticationModes returns the list of supported authentication modes for the selected broker depending on session info.
func (b *Broker) GetAuthenticationModes(ctx context.Context, sessionID string, supportedUILayouts []map[string]string) (authenticationModes []map[string]string, err error) {
	sessionInfo, err := b.sessionInfo(sessionID)
	if err != nil {
		return nil, err
	}

	//var candidatesAuthenticationModes []map[string]string
	allModes := getSupportedModes(sessionInfo, supportedUILayouts)

	// If the user needs mfa, we remove the last used mode from the list of available modes.
	if sessionInfo.currentMfaStep > 0 && sessionInfo.currentMfaStep < sessionInfo.neededMfa {
		allModes = getMfaModes(sessionInfo, sessionInfo.allModes)
	}
	// If the user needs or can reset the password, we only show those authentication modes.
	if sessionInfo.currentMfaStep > 0 && sessionInfo.pwdChange != noReset {
		allModes = getPasswdResetModes(sessionInfo, sessionInfo.allModes)
	}

	b.userLastSelectedModeMu.Lock()
	lastSelection := b.userLastSelectedMode[sessionInfo.username]
	b.userLastSelectedModeMu.Unlock()
	// Sort in preference order. We want by default password as first and potentially last selection too.
	if _, exists := allModes[lastSelection]; !exists {
		lastSelection = ""
	}

	var allModeIDs []string
	for n := range allModes {
		if n == "password" || n == lastSelection {
			continue
		}
		allModeIDs = append(allModeIDs, n)
	}
	sort.Strings(allModeIDs)

	if _, exists := allModes["password"]; exists {
		allModeIDs = append([]string{"password"}, allModeIDs...)
	}
	if lastSelection != "" && lastSelection != "password" {
		allModeIDs = append([]string{lastSelection}, allModeIDs...)
	}

	for _, id := range allModeIDs {
		authMode := allModes[id]
		authenticationModes = append(authenticationModes, map[string]string{
			"id":    id,
			"label": authMode["selection_label"],
		})
	}
	sessionInfo.allModes = allModes

	if err := b.updateSession(sessionID, sessionInfo); err != nil {
		return nil, err
	}

	return authenticationModes, nil
}

func getSupportedModes(sessionInfo sessionInfo, supportedUILayouts []map[string]string) map[string]map[string]string {
	allModes := make(map[string]map[string]string)
	for _, layout := range supportedUILayouts {
		switch layout["type"] {
		case "form":
			if layout["entry"] != "" {
				supportedEntries := strings.Split(strings.TrimPrefix(layout["entry"], "optional:"), ",")
				if slices.Contains(supportedEntries, "chars_password") {
					allModes["password"] = map[string]string{
						"selection_label": "Password authentication",
						"ui": mapToJSON(map[string]string{
							"type":  "form",
							"label": "Gimme your password",
							"entry": "chars_password",
						}),
					}
				}
				if slices.Contains(supportedEntries, "digits") {
					allModes["pincode"] = map[string]string{
						"selection_label": "Pin code",
						"ui": mapToJSON(map[string]string{
							"type":  "form",
							"label": "Enter your pin code",
							"entry": "digits",
						}),
					}
				}
				if slices.Contains(supportedEntries, "chars") && layout["wait"] != "" {
					allModes[fmt.Sprintf("entry_or_wait_for_%s_gmail.com", sessionInfo.username)] = map[string]string{
						"selection_label": fmt.Sprintf("Send URL to %s@gmail.com", sessionInfo.username),
						"email":           fmt.Sprintf("%s@gmail.com", sessionInfo.username),
						"ui": mapToJSON(map[string]string{
							"type":  "form",
							"label": fmt.Sprintf("Click on the link received at %s@gmail.com or enter the code:", sessionInfo.username),
							"entry": "chars",
							"wait":  "true",
						}),
					}
				}
			}

			// The broker could parse the values, that are either true/false
			if layout["wait"] != "" {
				if layout["button"] == "optional" {
					allModes["totp_with_button"] = map[string]string{
						"selection_label": "Authentication code",
						"phone":           "+33…",
						"wantedCode":      "temporary pass",
						"ui": mapToJSON(map[string]string{
							"type":   "form",
							"label":  "Enter your one time credential",
							"entry":  "chars",
							"button": "Resend sms",
						}),
					}
				} else {
					allModes["totp"] = map[string]string{
						"selection_label": "Authentication code",
						"phone":           "+33…",
						"wantedCode":      "temporary pass",
						"ui": mapToJSON(map[string]string{
							"type":  "form",
							"label": "Enter your one time credential",
							"entry": "chars",
						}),
					}
				}

				allModes["phoneack1"] = map[string]string{
					"selection_label": "Use your phone +33…",
					"phone":           "+33…",
					"ui": mapToJSON(map[string]string{
						"type":  "form",
						"label": "Unlock your phone +33… or accept request on web interface:",
						"wait":  "true",
					}),
				}

				allModes["phoneack2"] = map[string]string{
					"selection_label": "Use your phone +1…",
					"phone":           "+1…",
					"ui": mapToJSON(map[string]string{
						"type":  "form",
						"label": "Unlock your phone +1… or accept request on web interface",
						"wait":  "true",
					}),
				}

				allModes["fidodevice1"] = map[string]string{
					"selection_label": "Use your fido device foo",
					"ui": mapToJSON(map[string]string{
						"type":  "form",
						"label": "Plug your fido device and press with your thumb",
						"wait":  "true",
					}),
				}
			}

		case "qrcode":
			allModes["qrcodewithtypo"] = map[string]string{
				"selection_label": "Use a QR code",
				"ui": mapToJSON(map[string]string{
					"type":   "qrcode",
					"label":  "Enter the following code after flashing the address: ",
					"wait":   "true",
					"button": "regenerate QR code",
				}),
			}

		case "webview":
			// This broker does not support webview

		case "newpassword":
			if layout["entry"] == "" {
				break
			}
			allModes["mandatoryreset"] = map[string]string{
				"selection_label": "Password reset (3 days until mandatory)",
				"ui": mapToJSON(map[string]string{
					"type":  "newpassword",
					"label": "Enter your new password",
					"entry": "chars_password",
				}),
			}

			if layout["skip-button"] != "" {
				allModes["optionalreset"] = map[string]string{
					"selection_label": "Password reset",
					"ui": mapToJSON(map[string]string{
						"type":        "newpassword",
						"label":       "Enter your new password",
						"entry":       "chars_password",
						"skip-button": "Skip",
					}),
				}
			}
		}
	}

	return allModes
}

func getMfaModes(info sessionInfo, supportedModes map[string]map[string]string) map[string]map[string]string {
	mfaModes := make(map[string]map[string]string)
	for _, mode := range []string{"phoneack1", "totp_with_button", "fidodevice1"} {
		if _, exists := supportedModes[mode]; exists && info.selectedMode != mode {
			mfaModes[mode] = supportedModes[mode]
		}
	}
	return mfaModes
}

func getPasswdResetModes(info sessionInfo, supportedModes map[string]map[string]string) map[string]map[string]string {
	passwdResetModes := make(map[string]map[string]string)

	var mode string
	switch info.pwdChange {
	case canReset:
		mode = "optionalreset"
	case mustReset:
		mode = "mandatoryreset"
	}
	if authMode, exists := supportedModes[mode]; exists {
		passwdResetModes[mode] = authMode
	}

	return passwdResetModes
}

// SelectAuthenticationMode returns the UI layout information for the selected authentication mode.
func (b *Broker) SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error) {
	// Ensure session ID is an active one.
	sessionInfo, err := b.sessionInfo(sessionID)
	if err != nil {
		return nil, err
	}

	authenticationMode, exists := sessionInfo.allModes[authenticationModeName]
	if !exists {
		return nil, fmt.Errorf("selected authentication mode %q does not exists", authenticationModeName)
	}

	// populate UI options based on selected authentication mode
	uiLayoutInfo = jsonToMap(authenticationMode["ui"])

	// The broker does extra "out of bound" connections when needed
	switch authenticationModeName {
	case "totp_with_button", "totp":
		// send sms to sessionInfo.allModes[authenticationModeName]["phone"]
		// add a 0 to simulate new code generation.
		authenticationMode["wantedCode"] = authenticationMode["wantedCode"] + "0"
		sessionInfo.allModes[authenticationModeName] = authenticationMode
	case "phoneack1", "phoneack2":
		// send request to sessionInfo.allModes[authenticationModeName]["phone"]
	case "fidodevice1":
		// start transaction with fideo device
	case "qrcodewithtypo":
		// generate the url and finish the prompt on the fly.
		//nolint:gosec // this is some example code not shipped in production
		i := rand.Intn(3)
		contents := []string{"https://ubuntu.com", "https://ubuntu-fr.org", "https://canonical.com"}
		uiLayoutInfo["content"] = contents[i]
		uiLayoutInfo["label"] = uiLayoutInfo["label"] + "1337"
	}

	// Store selected mode
	sessionInfo.selectedMode = authenticationModeName

	if err = b.updateSession(sessionID, sessionInfo); err != nil {
		return nil, err
	}

	return uiLayoutInfo, nil
}

// IsAuthenticated evaluates the provided authenticationData and returns the authentication status for the user.
func (b *Broker) IsAuthenticated(ctx context.Context, sessionID, authenticationData string) (access, data string, err error) {
	sessionInfo, err := b.sessionInfo(sessionID)
	if err != nil {
		return "", "", err
	}

	//authenticationData = decryptAES([]byte(brokerEncryptionKey), authenticationData)
	var authData map[string]string
	if authenticationData != "" {
		if err := json.Unmarshal([]byte(authenticationData), &authData); err != nil {
			return "", "", errors.New("authentication data is not a valid json value")
		}
	}

	// Handles the context that will be assigned for the IsAuthenticated handler
	b.isAuthenticatedCallsMu.Lock()
	if _, exists := b.isAuthenticatedCalls[sessionID]; exists {
		b.isAuthenticatedCallsMu.Unlock()
		return "", "", fmt.Errorf("IsAuthenticated already running for session %q", sessionID)
	}
	ctx, cancel := context.WithCancel(ctx)
	b.isAuthenticatedCalls[sessionID] = isAuthenticatedCtx{ctx, cancel}
	b.isAuthenticatedCallsMu.Unlock()

	// Cleans up the IsAuthenticated context when the call is done.
	defer func() {
		b.isAuthenticatedCallsMu.Lock()
		delete(b.isAuthenticatedCalls, sessionID)
		b.isAuthenticatedCallsMu.Unlock()
	}()

	access, data, err = b.handleIsAuthenticated(b.isAuthenticatedCalls[sessionID].ctx, sessionInfo, authData)
	if access == responses.AuthGranted {
		switch sessionInfo.username {
		case "user-needs-reset":
			fallthrough
		case "user-can-reset":
			fallthrough
		case "user-mfa":
			if sessionInfo.currentMfaStep < sessionInfo.neededMfa {
				sessionInfo.currentMfaStep++
				access = responses.AuthNext
			}
		}
	} else if access == responses.AuthRetry {
		sessionInfo.attemptsPerMode[sessionInfo.selectedMode]++
		if sessionInfo.attemptsPerMode[sessionInfo.selectedMode] >= maxAttempts {
			access = responses.AuthDenied
		}
	}

	// Store last successful authentication mode for this user in the broker.
	b.userLastSelectedModeMu.Lock()
	b.userLastSelectedMode[sessionInfo.username] = sessionInfo.selectedMode
	b.userLastSelectedModeMu.Unlock()

	if err = b.updateSession(sessionID, sessionInfo); err != nil {
		return responses.AuthDenied, "", err
	}

	return access, data, err
}

//nolint:unparam // This is an static example implementation, so we don't return an error other than nil.
func (b *Broker) handleIsAuthenticated(ctx context.Context, sessionInfo sessionInfo, authData map[string]string) (access, data string, err error) {
	// Note that the "wait" authentication can be cancelled and switch to another mode with a challenge.
	// Take into account the cancellation.
	switch sessionInfo.selectedMode {
	case "password":
		if authData["challenge"] != "goodpass" {
			return responses.AuthRetry, `{"message": "invalid password, should be goodpass"}`, nil
		}

	case "pincode":
		if authData["challenge"] != "4242" {
			return responses.AuthRetry, `{"message": "invalid pincode, should be 4242"}`, nil
		}

	case "totp_with_button", "totp":
		wantedCode := sessionInfo.allModes[sessionInfo.selectedMode]["wantedCode"]
		if authData["challenge"] != wantedCode {
			return responses.AuthRetry, `{"message": "invalid totp code"}`, nil
		}

	case "phoneack1":
		// TODO: should this be an error rather (not expected data from the PAM module?
		if authData["wait"] != "true" {
			return responses.AuthDenied, `{"message": "phoneack1 should have wait set to true"}`, nil
		}
		// Send notification to phone1 and wait on server signal to return if OK or not
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return responses.AuthCancelled, "", nil
		}

	case "phoneack2":
		if authData["wait"] != "true" {
			return responses.AuthDenied, `{"message": "phoneack2 should have wait set to true"}`, nil
		}

		// This one is failing remotely as an example
		select {
		case <-time.After(2 * time.Second):
			return responses.AuthDenied, `{"message": "Timeout reached"}`, nil
		case <-ctx.Done():
			return responses.AuthCancelled, "", nil
		}

	case "fidodevice1":
		if authData["wait"] != "true" {
			return responses.AuthDenied, `{"message": "fidodevice1 should have wait set to true"}`, nil
		}

		// simulate direct exchange with the FIDO device
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return responses.AuthCancelled, "", nil
		}

	case "qrcodewithtypo":
		if authData["wait"] != "true" {
			return responses.AuthDenied, `{"message": "qrcodewithtypo should have wait set to true"}`, nil
		}
		// Simulate connexion with remote server to check that the correct code was entered
		select {
		case <-time.After(4 * time.Second):
		case <-ctx.Done():
			return responses.AuthCancelled, "", nil
		}
	}

	// this case name was dynamically generated
	if strings.HasPrefix(sessionInfo.selectedMode, "entry_or_wait_for_") {
		// do we have a challenge sent or should we just wait?
		if authData["challenge"] != "" {
			// validate challenge given manually by the user
			if authData["challenge"] != "aaaaa" {
				return responses.AuthDenied, `{"message": "invalid challenge, should be aaaaa"}`, nil
			}
		} else if authData["wait"] == "true" {
			// we are simulating clicking on the url signal received by the broker
			// this can be cancelled to resend a challenge
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return responses.AuthCancelled, "", nil
			}
		} else {
			return responses.AuthDenied, `{"message": "challenge timeout "}`, nil
		}
	}

	name, exists := exampleUsers[sessionInfo.username]
	if !exists {
		return responses.AuthDenied, `{"message": "user not found"}`, nil
	}

	return responses.AuthGranted, fmt.Sprintf(`{"userinfo": %s}`, userInfoFromName(name)), nil
}

// EndSession ends the requested session and triggers the necessary clean up steps, if any.
func (b *Broker) EndSession(ctx context.Context, sessionID string) error {
	if _, err := b.sessionInfo(sessionID); err != nil {
		return err
	}

	// Checks if there is a isAuthenticated call running for this session and cancels it before ending the session.
	if _, exists := b.isAuthenticatedCalls[sessionID]; exists {
		b.CancelIsAuthenticated(ctx, sessionID)
	}

	b.currentSessionsMu.Lock()
	defer b.currentSessionsMu.Unlock()
	delete(b.currentSessions, sessionID)
	return nil
}

// CancelIsAuthenticated cancels the IsAuthenticated request for the specified session.
// If there is no pending IsAuthenticated call for the session, this is a no-op.
func (b *Broker) CancelIsAuthenticated(ctx context.Context, sessionID string) {
	b.isAuthenticatedCallsMu.Lock()
	defer b.isAuthenticatedCallsMu.Unlock()
	if _, exists := b.isAuthenticatedCalls[sessionID]; !exists {
		return
	}
	b.isAuthenticatedCalls[sessionID].cancelFunc()
	delete(b.isAuthenticatedCalls, sessionID)
}

func mapToJSON(input map[string]string) string {
	data, err := json.Marshal(input)
	if err != nil {
		panic(fmt.Sprintf("Invalid map data: %v", err))
	}
	return string(data)
}

func jsonToMap(data string) map[string]string {
	r := make(map[string]string)
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		panic(fmt.Sprintf("Invalid map data: %v", err))
	}
	return r
}

// decryptAES is just here to illustrate the encryption and decryption
// and in no way the right way to perform a secure encryption
//
// TODO: This has to be changed in the final implementation.
//
//nolint:unused // This function will be refactored still, but is not used for now.
func encryptAES(key []byte, plaintext string) string {
	// create cipher
	c, err := aes.NewCipher(key)
	if err != nil {
		panic("prototype")
	}

	// allocate space for ciphered data
	out := make([]byte, len(plaintext))

	// encrypt
	c.Encrypt(out, []byte(plaintext))

	// return hex string
	return hex.EncodeToString(out)
}

// decryptAES is just here to illustrate the encryption and decryption
// and in no way the right way to perform a secure encryption
//
// TODO: This has to be changed in the final implementation.
//
//nolint:unused // This function will be refactored still, but is not used for now.
func decryptAES(key []byte, ct string) string {
	ciphertext, _ := hex.DecodeString(ct)

	c, err := aes.NewCipher(key)
	if err != nil {
		fmt.Println(err)
		panic("prototype")
	}

	pt := make([]byte, len(ciphertext))
	c.Decrypt(pt, ciphertext)

	return string(pt[:])
}

// sessionInfo returns the session information for the specified session ID or an error if the session is not active.
func (b *Broker) sessionInfo(sessionID string) (sessionInfo, error) {
	b.currentSessionsMu.RLock()
	defer b.currentSessionsMu.RUnlock()
	session, active := b.currentSessions[sessionID]
	if !active {
		return sessionInfo{}, fmt.Errorf("%s is not a current transaction", sessionID)
	}
	return session, nil
}

// updateSession checks if the session is still active and updates the session info.
func (b *Broker) updateSession(sessionID string, info sessionInfo) error {
	// Checks if the session was ended in the meantime, otherwise we would just accidentally recreate it.
	if _, err := b.sessionInfo(sessionID); err != nil {
		return err
	}
	b.currentSessionsMu.Lock()
	defer b.currentSessionsMu.Unlock()
	b.currentSessions[sessionID] = info
	return nil
}

// userInfoFromName transform a given name to the strinfigy userinfo string.
func userInfoFromName(name string) string {
	user := struct {
		Name string
	}{Name: name}

	var buf bytes.Buffer

	// only used for the example, we can ignore the template execution error as the returned data will be failing.
	_ = template.Must(template.New("").Parse(`{
		"name": "{{.Name}}",
		"uuid": "uuid-{{.Name}}",
		"gecos": "gecos for {{.Name}}",
		"dir": "/home/{{.Name}}",
		"shell": "/bin/sh/{{.Name}}",
		"avatar": "avatar for {{.Name}}",
		"groups": [ {"name": "group-{{.Name}}", "ugid": "group-{{.Name}}"} ]
	}`)).Execute(&buf, user)

	return buf.String()
}
