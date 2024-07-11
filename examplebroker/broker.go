// Package examplebroker implements an example broker that will be used by the authentication daemon.
package examplebroker

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
)

const maxAttempts int = 5

type passwdReset int

const (
	noReset passwdReset = iota
	canReset
	mustReset
)

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

type sessionInfo struct {
	username    string
	lang        string
	sessionMode string

	currentAuthMode string
	allModes        map[string]map[string]string
	attemptsPerMode map[string]int

	pwdChange passwdReset

	neededAuthSteps   int
	currentAuthStep   int
	firstSelectedMode string

	qrcodeSelections int
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

	privateKey *rsa.PrivateKey
}

type userInfoBroker struct {
	Password string
}

var (
	exampleUsersMu = sync.RWMutex{}
	exampleUsers   = map[string]userInfoBroker{
		"user1":                 {Password: "goodpass"},
		"user2":                 {Password: "goodpass"},
		"user3":                 {Password: "goodpass"},
		"user-mfa":              {Password: "goodpass"},
		"user-mfa-with-reset":   {Password: "goodpass"},
		"user-needs-reset":      {Password: "goodpass"},
		"user-can-reset":        {Password: "goodpass"},
		"user-can-reset2":       {Password: "goodpass"},
		"user-local-groups":     {Password: "goodpass"},
		"user-pre-check":        {Password: "goodpass"},
		"user-sudo":             {Password: "goodpass"},
		"user-mismatching-name": {Password: "goodpass"},
	}
)

// New creates a new examplebroker object.
func New(name string) (b *Broker, fullName, brandIcon string) {
	// Generate a new private key for the broker.
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("could not create an valid rsa key: %v", err))
	}

	return &Broker{
		currentSessions:        make(map[string]sessionInfo),
		currentSessionsMu:      sync.RWMutex{},
		userLastSelectedMode:   make(map[string]string),
		userLastSelectedModeMu: sync.Mutex{},
		isAuthenticatedCalls:   make(map[string]isAuthenticatedCtx),
		isAuthenticatedCallsMu: sync.Mutex{},
		privateKey:             privateKey,
	}, strings.ReplaceAll(name, "_", " "), fmt.Sprintf("/usr/share/brokers/%s.png", name)
}

// NewSession creates a new session for the specified user.
func (b *Broker) NewSession(ctx context.Context, username, lang, mode string) (sessionID, encryptionKey string, err error) {
	sessionID = uuid.New().String()
	info := sessionInfo{
		username:        username,
		lang:            lang,
		sessionMode:     mode,
		pwdChange:       noReset,
		currentAuthStep: 1,
		neededAuthSteps: 1,
		attemptsPerMode: make(map[string]int),
	}

	switch username {
	case "user-mfa":
		info.neededAuthSteps = 3
	case "user-needs-reset":
		info.neededAuthSteps = 2
		info.pwdChange = mustReset
	case "user-can-reset":
		fallthrough
	case "user-can-reset2":
		info.neededAuthSteps = 2
		info.pwdChange = canReset
	case "user-mfa-with-reset":
		info.neededAuthSteps = 3
		info.pwdChange = canReset
	case "user-unexistent":
		return "", "", fmt.Errorf("user %q does not exist", username)
	}

	if info.sessionMode == "passwd" {
		info.neededAuthSteps++
		info.pwdChange = mustReset
	}

	exampleUsersMu.Lock()
	defer exampleUsersMu.Unlock()
	if _, ok := exampleUsers[username]; !ok && strings.HasPrefix(username, "user-integration") {
		exampleUsers[username] = userInfoBroker{Password: "goodpass"}
	}

	if _, ok := exampleUsers[username]; !ok && strings.HasPrefix(username, "user-mfa-integration") {
		exampleUsers[username] = userInfoBroker{Password: "goodpass"}
		info.neededAuthSteps = 3
	}

	if _, ok := exampleUsers[username]; !ok && strings.HasPrefix(username, "user-needs-reset-integration") {
		exampleUsers[username] = userInfoBroker{Password: "goodpass"}
		info.neededAuthSteps = 2
		info.pwdChange = mustReset
	}

	pubASN1, err := x509.MarshalPKIXPublicKey(&b.privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}

	b.currentSessionsMu.Lock()
	b.currentSessions[sessionID] = info
	b.currentSessionsMu.Unlock()
	return sessionID, base64.StdEncoding.EncodeToString(pubASN1), nil
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
	if sessionInfo.currentAuthStep > 1 && sessionInfo.currentAuthStep <= sessionInfo.neededAuthSteps {
		allModes = getMfaModes(sessionInfo, sessionInfo.allModes)
	}
	// If the user needs or can reset the password, we only show those authentication modes.
	if sessionInfo.currentAuthStep == sessionInfo.neededAuthSteps && sessionInfo.pwdChange != noReset {
		if sessionInfo.currentAuthStep < 2 {
			return nil, fmt.Errorf("password reset is not allowed before authentication")
		}

		allModes = getPasswdResetModes(sessionInfo, supportedUILayouts)
		if sessionInfo.pwdChange == mustReset && len(allModes) == 0 {
			return nil, fmt.Errorf("user %q must reset password, but no mode was provided for it", sessionInfo.username)
		}
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
			modeName := "qrcodewithtypo"
			modeLabel := "Enter the following code after flashing the address: "
			if layout["code"] != "" {
				modeName = "qrcodeandcodewithtypo"
				modeLabel = "Scan the qrcode or enter the code in the login page"
			}
			allModes[modeName] = map[string]string{
				"selection_label": "Use a QR code",
				"ui": mapToJSON(map[string]string{
					"type":   "qrcode",
					"label":  modeLabel,
					"wait":   "true",
					"button": "Regenerate code",
				}),
			}

		case "webview":
			// This broker does not support webview
		}
	}

	return allModes
}

func getMfaModes(info sessionInfo, supportedModes map[string]map[string]string) map[string]map[string]string {
	mfaModes := make(map[string]map[string]string)
	for _, mode := range []string{"phoneack1", "totp_with_button", "fidodevice1"} {
		if _, exists := supportedModes[mode]; exists && info.currentAuthMode != mode {
			mfaModes[mode] = supportedModes[mode]
		}
	}
	return mfaModes
}

func getPasswdResetModes(info sessionInfo, supportedUILayouts []map[string]string) map[string]map[string]string {
	passwdResetModes := make(map[string]map[string]string)
	for _, layout := range supportedUILayouts {
		if layout["type"] != "newpassword" {
			continue
		}
		if layout["entry"] == "" {
			break
		}

		ui := map[string]string{
			"type":  "newpassword",
			"label": "Enter your new password",
			"entry": "chars_password",
		}

		mode := "mandatoryreset"
		if info.pwdChange == canReset && layout["button"] != "" {
			mode = "optionalreset"
			ui["label"] = "Enter your new password (3 days until mandatory)"
			ui["button"] = "Skip"
		}

		passwdResetModes[mode] = map[string]string{
			"selection_label": "Password reset",
			"ui":              mapToJSON(ui),
		}
	}
	return passwdResetModes
}

func qrcodeData(sessionInfo *sessionInfo) (content string, code string) {
	baseCode := 1337
	qrcodeURIs := []string{
		"https://ubuntu.com",
		"https://ubuntu.fr/",
		"https://ubuntuforum-br.org/",
		"https://www.ubuntu-it.org/",
	}

	if strings.HasPrefix(sessionInfo.username, "user-integration-qrcode-static") {
		return qrcodeURIs[0], fmt.Sprint(baseCode)
	}

	defer func() { sessionInfo.qrcodeSelections++ }()
	return qrcodeURIs[sessionInfo.qrcodeSelections%len(qrcodeURIs)],
		fmt.Sprint(baseCode + sessionInfo.qrcodeSelections)
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
		// start transaction with fido device
	case "qrcodeandcodewithtypo":
		uiLayoutInfo["content"], uiLayoutInfo["code"] = qrcodeData(&sessionInfo)
	case "qrcodewithtypo":
		// generate the url and finish the prompt on the fly.
		content, code := qrcodeData(&sessionInfo)
		uiLayoutInfo["label"] += code
		uiLayoutInfo["content"] = content
	}

	// Store selected mode
	sessionInfo.currentAuthMode = authenticationModeName
	// Store the first one to use to update the lastSelectedMode in MFA cases.
	if sessionInfo.currentAuthStep == 1 {
		sessionInfo.firstSelectedMode = authenticationModeName
	}

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

	access, data, err = b.handleIsAuthenticated(ctx, sessionInfo, authData)
	if access == AuthGranted && sessionInfo.currentAuthStep < sessionInfo.neededAuthSteps {
		sessionInfo.currentAuthStep++
		access = AuthNext
		data = ""
	} else if access == AuthRetry {
		sessionInfo.attemptsPerMode[sessionInfo.currentAuthMode]++
		if sessionInfo.attemptsPerMode[sessionInfo.currentAuthMode] >= maxAttempts {
			access = AuthDenied
		}
	}

	// Store last successful authentication mode for this user in the broker.
	if access == AuthGranted {
		b.userLastSelectedModeMu.Lock()
		b.userLastSelectedMode[sessionInfo.username] = sessionInfo.firstSelectedMode
		b.userLastSelectedModeMu.Unlock()
	}

	if err = b.updateSession(sessionID, sessionInfo); err != nil {
		return AuthDenied, "", err
	}

	return access, data, err
}

func (b *Broker) handleIsAuthenticated(ctx context.Context, sessionInfo sessionInfo, authData map[string]string) (access, data string, err error) {
	// Decrypt challenge if present.
	challenge, err := decodeRawChallenge(b.privateKey, authData["challenge"])
	if err != nil {
		return AuthRetry, fmt.Sprintf(`{"message": "could not decode challenge: %v"}`, err), nil
	}

	// Note that the "wait" authentication can be cancelled and switch to another mode with a challenge.
	// Take into account the cancellation.
	switch sessionInfo.currentAuthMode {
	case "password":
		exampleUsersMu.RLock()
		defer exampleUsersMu.RUnlock()
		expectedChallenge := exampleUsers[sessionInfo.username].Password

		if challenge != expectedChallenge {
			return AuthRetry, fmt.Sprintf(`{"message": "invalid password '%s', should be '%s'"}`, challenge, expectedChallenge), nil
		}

	case "pincode":
		if challenge != "4242" {
			return AuthRetry, `{"message": "invalid pincode, should be 4242"}`, nil
		}

	case "totp_with_button", "totp":
		wantedCode := sessionInfo.allModes[sessionInfo.currentAuthMode]["wantedCode"]
		if challenge != wantedCode {
			return AuthRetry, `{"message": "invalid totp code"}`, nil
		}

	case "phoneack1":
		// TODO: should this be an error rather (not expected data from the PAM module?
		if authData["wait"] != "true" {
			return AuthDenied, `{"message": "phoneack1 should have wait set to true"}`, nil
		}
		// Send notification to phone1 and wait on server signal to return if OK or not
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return AuthCancelled, "", nil
		}

	case "phoneack2":
		if authData["wait"] != "true" {
			return AuthDenied, `{"message": "phoneack2 should have wait set to true"}`, nil
		}

		// This one is failing remotely as an example
		select {
		case <-time.After(2 * time.Second):
			return AuthDenied, `{"message": "Timeout reached"}`, nil
		case <-ctx.Done():
			return AuthCancelled, "", nil
		}

	case "fidodevice1":
		if authData["wait"] != "true" {
			return AuthDenied, `{"message": "fidodevice1 should have wait set to true"}`, nil
		}

		// simulate direct exchange with the FIDO device
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return AuthCancelled, "", nil
		}

	case "qrcodewithtypo", "qrcodeandcodewithtypo":
		if authData["wait"] != "true" {
			return AuthDenied, fmt.Sprintf(`{"message": "%s should have wait set to true"}`, sessionInfo.currentAuthMode), nil
		}
		// Simulate connexion with remote server to check that the correct code was entered
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return AuthCancelled, "", nil
		}

	case "optionalreset":
		if authData["skip"] == "true" {
			break
		}
		fallthrough
	case "mandatoryreset":
		exampleUsersMu.Lock()
		defer exampleUsersMu.Unlock()

		expectedChallenge := "authd2404"
		// Reset the password to default if it had already been changed.
		// As at PAM level we'd refuse a previous password to be re-used.
		if exampleUsers[sessionInfo.username].Password == expectedChallenge {
			expectedChallenge = "goodpass"
		}

		if challenge != expectedChallenge {
			return AuthRetry, fmt.Sprintf(`{"message": "new password does not match criteria: must be '%s'"}`, expectedChallenge), nil
		}
		exampleUsers[sessionInfo.username] = userInfoBroker{Password: challenge}
	}

	// this case name was dynamically generated
	if strings.HasPrefix(sessionInfo.currentAuthMode, "entry_or_wait_for_") {
		// do we have a challenge sent or should we just wait?
		if challenge != "" {
			// validate challenge given manually by the user
			if challenge != "aaaaa" {
				return AuthDenied, `{"message": "invalid challenge, should be aaaaa"}`, nil
			}
		} else if authData["wait"] == "true" {
			// we are simulating clicking on the url signal received by the broker
			// this can be cancelled to resend a challenge
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return AuthCancelled, "", nil
			}
		} else {
			return AuthDenied, `{"message": "challenge timeout "}`, nil
		}
	}

	if _, exists := exampleUsers[sessionInfo.username]; !exists {
		return AuthDenied, `{"message": "user not found"}`, nil
	}
	return AuthGranted, fmt.Sprintf(`{"userinfo": %s}`, userInfoFromName(sessionInfo.username)), nil
}

// decodeRawChallenge extract the base64 challenge and try to decrypt it with the private key.
func decodeRawChallenge(priv *rsa.PrivateKey, rawChallenge string) (string, error) {
	if rawChallenge == "" {
		return "", nil
	}

	ciphertext, err := base64.StdEncoding.DecodeString(rawChallenge)
	if err != nil {
		return "", err
	}

	plaintext, err := rsa.DecryptOAEP(sha512.New(), nil, priv, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
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

// UserPreCheck checks if the user is known to the broker.
func (b *Broker) UserPreCheck(ctx context.Context, username string) error {
	if _, exists := exampleUsers[username]; !exists {
		return fmt.Errorf("user %q does not exist", username)
	}
	return nil
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
	type groupJSONInfo struct {
		Name string
		UGID string
	}

	user := struct {
		Name   string
		UUID   string
		Home   string
		Shell  string
		Groups []groupJSONInfo
		Gecos  string
	}{
		Name:   name,
		UUID:   "uuid-" + name,
		Home:   "/home/" + name,
		Shell:  "/usr/bin/bash",
		Groups: []groupJSONInfo{{Name: "group-" + name, UGID: "ugid-" + name}},
		Gecos:  "gecos for " + name,
	}

	switch name {
	case "user-local-groups":
		user.Groups = append(user.Groups, groupJSONInfo{Name: "localgroup", UGID: ""})

	case "user-sudo":
		user.Groups = append(user.Groups, groupJSONInfo{Name: "sudo", UGID: ""}, groupJSONInfo{Name: "admin", UGID: ""})

	case "user-mismatching-name":
		user.Name = "mismatching-username"
	}

	// only used for tests, we can ignore the template execution error as the returned data will be failing.
	var buf bytes.Buffer
	_ = template.Must(template.New("").Parse(`{
		"name": "{{.Name}}",
		"uuid": "{{.UUID}}",
		"gecos": "{{.Gecos}}",
		"dir": "{{.Home}}",
		"shell": "{{.Shell}}",
		"groups": [ {{range $index, $g := .Groups}}
			{{- if $index}}, {{end -}}
			{"name": "{{.Name}}", "ugid": "{{.UGID}}"}
		{{- end}} ]
	}`)).Execute(&buf, user)

	return buf.String()
}
