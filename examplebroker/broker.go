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
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ubuntu/authd/brokers/auth"
	"github.com/ubuntu/authd/brokers/layouts"
	"github.com/ubuntu/authd/brokers/layouts/entries"
	"github.com/ubuntu/authd/internal/log"
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
	optionalResetMode  = "optionalreset"
	mandatoryResetMode = "mandatoryreset"
)

type authMode struct {
	id             string
	selectionLabel string
	ui             *layouts.UILayout
	email          string
	phone          string
	wantedCode     string
	isMFA          bool
}

type sessionInfo struct {
	username    string
	lang        string
	sessionMode string

	currentAuthMode string
	allModes        map[string]authMode
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

	sleepMultiplier float64
}

type userInfoBroker struct {
	Password string
}

var (
	exampleUsersMu = sync.RWMutex{}
	exampleUsers   = map[string]userInfoBroker{
		"user1":               {Password: "goodpass"},
		"user2":               {Password: "goodpass"},
		"user3":               {Password: "goodpass"},
		"user-mfa":            {Password: "goodpass"},
		"user-mfa-with-reset": {Password: "goodpass"},
		"user-needs-reset":    {Password: "goodpass"},
		"user-needs-reset2":   {Password: "goodpass"},
		"user-can-reset":      {Password: "goodpass"},
		"user-can-reset2":     {Password: "goodpass"},
		"user-local-groups":   {Password: "goodpass"},
		"user-pre-check":      {Password: "goodpass"},
		"user-sudo":           {Password: "goodpass"},
	}
)

var (
	passwordMode = authMode{
		id:             "password",
		selectionLabel: "Password authentication",
		ui: layouts.NewUI(
			layouts.UIForm,
			layouts.WithLabel("Gimme your password"),
			layouts.WithEntry(entries.CharsPassword),
		),
	}

	pinCodeMode = authMode{
		id:             "pincode",
		selectionLabel: "Pin code",
		ui: layouts.NewUI(
			layouts.UIForm,
			layouts.WithLabel("Enter your pin code"),
			layouts.WithEntry(entries.Digits),
		),
	}

	totpMode = authMode{
		id:             "totp",
		selectionLabel: "Authentication code",
		phone:          "+33...",
		wantedCode:     "temporary pass",
		ui: layouts.NewUI(
			layouts.UIForm,
			layouts.WithLabel("Enter your one time credential"),
			layouts.WithEntry(entries.Chars),
		),
	}

	totpWithButtonMode = authMode{
		id:             "totp_with_button",
		selectionLabel: "Authentication code",
		phone:          "+33...",
		wantedCode:     "temporary pass",
		isMFA:          true,
		ui: layouts.NewUI(
			layouts.UIForm,
			layouts.WithLabel("Enter your one time credential"),
			layouts.WithEntry(entries.Chars),
			layouts.WithButton("Resend sms"),
		),
	}

	phoneAck1Mode = authMode{
		id:             "phoneack1",
		selectionLabel: "Use your phone +33...",
		phone:          "+33...",
		isMFA:          true,
		ui: layouts.NewUI(
			layouts.UIForm,
			layouts.WithLabel("Unlock your phone +33... or accept request on web interface:"),
			layouts.WithWaitBool(true),
		),
	}

	phoneAck2Mode = authMode{
		id:             "phoneack2",
		selectionLabel: "Use your phone +1...",
		phone:          "+1...",
		ui: layouts.NewUI(
			layouts.UIForm,
			layouts.WithLabel("Unlock your phone +1... or accept request on web interface"),
			layouts.WithWaitBool(true),
		),
	}

	fidoDeviceMode = authMode{
		id:             "fidodevice1",
		selectionLabel: "Use your fido device foo",
		isMFA:          true,
		ui: layouts.NewUI(
			layouts.UIForm,
			layouts.WithLabel("Plug your fido device and press with your thumb"),
			layouts.WithWaitBool(true),
		),
	}

	emailMode = func(userName string) authMode {
		return authMode{
			id:             fmt.Sprintf("entry_or_wait_for_%s_gmail.com", userName),
			selectionLabel: fmt.Sprintf("Send URL to %s@gmail.com", userName),
			email:          fmt.Sprintf("%s@gmail.com", userName),
			ui: layouts.NewUI(
				layouts.UIForm,
				layouts.WithLabel(fmt.Sprintf("Click on the link received at %s@gmail.com or enter the code:",
					userName)),
				layouts.WithEntry(entries.Chars),
				layouts.WithWaitBool(true),
			),
		}
	}

	qrCodeModeBase = func(id, selectionLabel, label string) authMode {
		return authMode{
			id:             id,
			selectionLabel: selectionLabel,
			ui: layouts.NewUI(
				layouts.UIQrCode,
				layouts.WithLabel(label),
				layouts.WithWaitBool(true),
				layouts.WithButton("Regenerate code"),
			),
		}
	}

	qrCodeMode = qrCodeModeBase("qrcodewithtypo", "Use a QR code",
		"Enter the following code after flashing the address: ")

	qrCodeAndCodeMode = qrCodeModeBase("qrcodeandcodewithtypo", "Use a QR code",
		"Scan the qrcode or enter the code in the login page")

	codeMode = qrCodeModeBase("codewithtypo", "Use a Login code",
		"Enter the code in the login page")

	// Not implemented yet.
	webViewMode = authMode{
		id: "webview",
	}
)

// New creates a new examplebroker object.
func New(name string) (b *Broker, fullName, brandIcon string) {
	// Generate a new private key for the broker.
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("could not create an valid rsa key: %v", err))
	}

	sleepMultiplier := 1.0
	if v := os.Getenv("AUTHD_EXAMPLE_BROKER_SLEEP_MULTIPLIER"); v != "" {
		var err error
		sleepMultiplier, err = strconv.ParseFloat(v, 64)
		if err != nil {
			panic(err)
		}
		if sleepMultiplier <= 0 {
			panic("Negative or 0 sleep multiplier is not supported")
		}
	}

	log.Debugf(context.TODO(), "Using sleep multiplier: %f", sleepMultiplier)

	return &Broker{
		currentSessions:        make(map[string]sessionInfo),
		currentSessionsMu:      sync.RWMutex{},
		userLastSelectedMode:   make(map[string]string),
		userLastSelectedModeMu: sync.Mutex{},
		isAuthenticatedCalls:   make(map[string]isAuthenticatedCtx),
		isAuthenticatedCallsMu: sync.Mutex{},
		privateKey:             privateKey,
		sleepMultiplier:        sleepMultiplier,
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
		fallthrough
	case "user-needs-reset2":
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

	if info.sessionMode == auth.SessionModePasswd {
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

	if _, ok := exampleUsers[username]; !ok && strings.HasPrefix(username, "user-mfa-needs-reset-integration") {
		exampleUsers[username] = userInfoBroker{Password: "goodpass"}
		info.neededAuthSteps = 3
		info.pwdChange = mustReset
	}

	if _, ok := exampleUsers[username]; !ok && strings.HasPrefix(username, "user-mfa-with-reset-integration") {
		exampleUsers[username] = userInfoBroker{Password: "goodpass"}
		info.neededAuthSteps = 3
		info.pwdChange = canReset
	}

	if _, ok := exampleUsers[username]; !ok && strings.HasPrefix(username, "user-needs-reset-integration") {
		exampleUsers[username] = userInfoBroker{Password: "goodpass"}
		info.neededAuthSteps = 2
		info.pwdChange = mustReset
	}

	if _, ok := exampleUsers[username]; !ok && strings.HasPrefix(username, "user-can-reset-integration") {
		exampleUsers[username] = userInfoBroker{Password: "goodpass"}
		info.neededAuthSteps = 2
		info.pwdChange = canReset
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

	log.Debugf(ctx, "Supported UI layouts by %s, %#v", sessionID, supportedUILayouts)
	allModes := getSupportedModes(sessionInfo, supportedUILayouts)

	// If the user needs mfa, we remove the last used mode from the list of available modes.
	if sessionInfo.currentAuthStep > 1 && sessionInfo.currentAuthStep <= sessionInfo.neededAuthSteps {
		allModes = getMfaModes(sessionInfo, sessionInfo.allModes)
	}
	// If the user needs or can reset the password, we only show those authentication modes.
	if sessionInfo.currentAuthStep == sessionInfo.neededAuthSteps && sessionInfo.pwdChange != noReset {
		if sessionInfo.currentAuthStep < 2 {
			return nil, errors.New("password reset is not allowed before authentication")
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
		if n == passwordMode.id || n == lastSelection {
			continue
		}
		allModeIDs = append(allModeIDs, n)
	}
	sort.Strings(allModeIDs)

	if _, exists := allModes[passwordMode.id]; exists {
		allModeIDs = append([]string{passwordMode.id}, allModeIDs...)
	}
	if lastSelection != "" && lastSelection != passwordMode.id {
		allModeIDs = append([]string{lastSelection}, allModeIDs...)
	}

	for _, id := range allModeIDs {
		authMode := allModes[id]
		authenticationModes = append(authenticationModes, map[string]string{
			layouts.ID:    id,
			layouts.Label: authMode.selectionLabel,
		})
	}
	log.Debugf(ctx, "Supported authentication modes for %s: %#v", sessionID, allModes)
	sessionInfo.allModes = allModes

	if err := b.updateSession(sessionID, sessionInfo); err != nil {
		return nil, err
	}

	return authenticationModes, nil
}

func getSupportedModes(sessionInfo sessionInfo, supportedUILayouts []map[string]string) map[string]authMode {
	allModes := make(map[string]authMode)
	for _, layout := range supportedUILayouts {
		switch layout[layouts.Type] {
		case layouts.Form:
			if layout[layouts.Entry] != "" {
				_, supportedEntries := layouts.ParseItems(layout[layouts.Entry])
				if slices.Contains(supportedEntries, entries.CharsPassword) {
					allModes[passwordMode.id] = passwordMode
				}
				if slices.Contains(supportedEntries, entries.Digits) {
					allModes[pinCodeMode.id] = pinCodeMode
				}
				if slices.Contains(supportedEntries, entries.Chars) && layout[layouts.Wait] != "" {
					mode := emailMode(sessionInfo.username)
					allModes[mode.id] = mode
				}
			}

			// The broker could parse the values, that are either true/false
			if layout[layouts.Wait] != "" {
				if layout[layouts.Button] == layouts.Optional {
					allModes[totpWithButtonMode.id] = totpWithButtonMode
				} else {
					allModes[totpMode.id] = totpMode
				}

				allModes[phoneAck1Mode.id] = phoneAck1Mode
				allModes[phoneAck2Mode.id] = phoneAck2Mode
				allModes[fidoDeviceMode.id] = fidoDeviceMode
			}

		case layouts.QrCode:
			mode := qrCodeMode
			if layout[layouts.Code] != "" {
				mode = qrCodeAndCodeMode
			}
			if layout[layouts.RendersQrCode] != layouts.True {
				mode = codeMode
			}
			allModes[mode.id] = mode

		case webViewMode.id:
			// This broker does not support webview
		}
	}

	return allModes
}

func getMfaModes(info sessionInfo, supportedModes map[string]authMode) map[string]authMode {
	mfaModes := make(map[string]authMode)
	for _, mode := range supportedModes {
		if !mode.isMFA {
			continue
		}
		if info.currentAuthMode == mode.id {
			continue
		}
		mfaModes[mode.id] = mode
	}
	return mfaModes
}

func getPasswdResetModes(info sessionInfo, supportedUILayouts []map[string]string) map[string]authMode {
	passwdResetModes := make(map[string]authMode)
	for _, layout := range supportedUILayouts {
		if layout[layouts.Type] != layouts.NewPassword {
			continue
		}
		if layout[layouts.Entry] == "" {
			break
		}

		layoutOpts := []layouts.UIOptions{
			layouts.WithLabel("Enter your new password"),
			layouts.WithEntry(entries.CharsPassword),
		}

		mode := mandatoryResetMode
		if info.pwdChange == canReset && layout[layouts.Button] != "" {
			mode = optionalResetMode
			layoutOpts = append(layoutOpts,
				layouts.WithLabel("Enter your new password (3 days until mandatory)"),
				layouts.WithButton("Skip"),
			)
		}

		passwdResetModes[mode] = authMode{
			selectionLabel: "Password reset",
			ui:             layouts.NewUI(layouts.UINewPassword, layoutOpts...),
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

func cloneLayout(l *layouts.UILayout, opts ...layouts.UIOptions) (*layouts.UILayout, error) {
	asMap, err := l.ToMap()
	if err != nil {
		return nil, err
	}
	cloned, err := layouts.NewUIFromMap(asMap)
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(cloned)
	}

	return cloned, nil
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
	uiLayout := authenticationMode.ui

	// The broker does extra "out of bound" connections when needed
	switch authenticationModeName {
	case totpWithButtonMode.id, totpMode.id:
		// send sms to sessionInfo.allModes[authenticationModeName].phone
		// add a 0 to simulate new code generation.
		authenticationMode.wantedCode += "0"
		sessionInfo.allModes[authenticationModeName] = authenticationMode
	case phoneAck1Mode.id, phoneAck2Mode.id:
		// send request to sessionInfo.allModes[authenticationModeName].phone
	case fidoDeviceMode.id:
		// start transaction with fido device
	case qrCodeAndCodeMode.id, codeMode.id:
		content, code := qrcodeData(&sessionInfo)
		uiLayout, err = cloneLayout(uiLayout,
			layouts.WithCode(code), layouts.WithContent(content))
		if err != nil {
			return nil, err
		}
	case qrCodeMode.id:
		// generate the url and finish the prompt on the fly.
		content, code := qrcodeData(&sessionInfo)
		uiLayout, err = cloneLayout(uiLayout,
			layouts.WithLabel(uiLayout.GetLabel()+code),
			layouts.WithContent(content))
		if err != nil {
			return nil, err
		}
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

	return uiLayout.ToMap()
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

	access, data = b.handleIsAuthenticated(ctx, sessionInfo, authData)
	if access == auth.Granted && sessionInfo.currentAuthStep < sessionInfo.neededAuthSteps {
		sessionInfo.currentAuthStep++
		access = auth.Next
		data = ""
	} else if access == auth.Retry {
		sessionInfo.attemptsPerMode[sessionInfo.currentAuthMode]++
		if sessionInfo.attemptsPerMode[sessionInfo.currentAuthMode] >= maxAttempts {
			access = auth.Denied
		}
	}

	// Store last successful authentication mode for this user in the broker.
	if access == auth.Granted {
		b.userLastSelectedModeMu.Lock()
		b.userLastSelectedMode[sessionInfo.username] = sessionInfo.firstSelectedMode
		b.userLastSelectedModeMu.Unlock()
	}

	if err = b.updateSession(sessionID, sessionInfo); err != nil {
		return auth.Denied, "", err
	}

	return access, data, err
}

func (b *Broker) sleepDuration(in time.Duration) time.Duration {
	return time.Duration(math.Round(float64(in) * b.sleepMultiplier))
}

func (b *Broker) handleIsAuthenticated(ctx context.Context, sessionInfo sessionInfo, authData map[string]string) (access, data string) {
	// Decrypt challenge if present.
	challenge, err := decodeRawChallenge(b.privateKey, authData["challenge"])
	if err != nil {
		return auth.Retry, fmt.Sprintf(`{"message": "could not decode challenge: %v"}`, err)
	}

	exampleUsersMu.Lock()
	user, userExists := exampleUsers[sessionInfo.username]
	exampleUsersMu.Unlock()
	if !userExists {
		return auth.Denied, `{"message": "user not found"}`
	}

	sleepDuration := b.sleepDuration(4 * time.Second)

	// Note that the layouts.Wait authentication can be cancelled and switch to another mode with a challenge.
	// Take into account the cancellation.
	switch sessionInfo.currentAuthMode {
	case passwordMode.id:
		expectedChallenge := user.Password

		if challenge != expectedChallenge {
			return auth.Retry, fmt.Sprintf(`{"message": "invalid password '%s', should be '%s'"}`, challenge, expectedChallenge)
		}

	case pinCodeMode.id:
		if challenge != "4242" {
			return auth.Retry, `{"message": "invalid pincode, should be 4242"}`
		}

	case totpWithButtonMode.id, totpMode.id:
		wantedCode := sessionInfo.allModes[sessionInfo.currentAuthMode].wantedCode
		if challenge != wantedCode {
			return auth.Retry, `{"message": "invalid totp code"}`
		}

	case phoneAck1Mode.id:
		// TODO: should this be an error rather (not expected data from the PAM module?
		if authData[layouts.Wait] != layouts.True {
			return auth.Denied, `{"message": "phoneack1 should have wait set to true"}`
		}
		// Send notification to phone1 and wait on server signal to return if OK or not
		select {
		case <-time.After(sleepDuration):
		case <-ctx.Done():
			return auth.Cancelled, ""
		}

	case phoneAck2Mode.id:
		if authData[layouts.Wait] != layouts.True {
			return auth.Denied, `{"message": "phoneack2 should have wait set to true"}`
		}

		// This one is failing remotely as an example
		select {
		case <-time.After(sleepDuration):
			return auth.Denied, `{"message": "Timeout reached"}`
		case <-ctx.Done():
			return auth.Cancelled, ""
		}

	case fidoDeviceMode.id:
		if authData[layouts.Wait] != layouts.True {
			return auth.Denied, `{"message": "fidodevice1 should have wait set to true"}`
		}

		// simulate direct exchange with the FIDO device
		select {
		case <-time.After(sleepDuration):
		case <-ctx.Done():
			return auth.Cancelled, ""
		}

	case qrCodeMode.id, qrCodeAndCodeMode.id, codeMode.id:
		if authData[layouts.Wait] != layouts.True {
			return auth.Denied, fmt.Sprintf(`{"message": "%s should have wait set to true"}`, sessionInfo.currentAuthMode)
		}
		// Simulate connexion with remote server to check that the correct code was entered
		select {
		case <-time.After(sleepDuration):
		case <-ctx.Done():
			return auth.Cancelled, ""
		}

	case optionalResetMode:
		if authData["skip"] == layouts.True {
			break
		}
		fallthrough
	case mandatoryResetMode:
		expectedChallenge := "authd2404"
		// Reset the password to default if it had already been changed.
		// As at PAM level we'd refuse a previous password to be re-used.
		if user.Password == expectedChallenge {
			expectedChallenge = "goodpass"
		}

		if challenge != expectedChallenge {
			return auth.Retry, fmt.Sprintf(`{"message": "new password does not match criteria: must be '%s'"}`, expectedChallenge)
		}
		exampleUsersMu.Lock()
		exampleUsers[sessionInfo.username] = userInfoBroker{Password: challenge}
		exampleUsersMu.Unlock()

	// this case name was dynamically generated
	case emailMode(sessionInfo.username).id:
		// do we have a challenge sent or should we just wait?
		if challenge != "" {
			// validate challenge given manually by the user
			if challenge != "aaaaa" {
				return auth.Denied, `{"message": "invalid challenge, should be aaaaa"}`
			}
		} else if authData[layouts.Wait] == layouts.True {
			// we are simulating clicking on the url signal received by the broker
			// this can be cancelled to resend a challenge
			select {
			case <-time.After(b.sleepDuration(10 * time.Second)):
			case <-ctx.Done():
				return auth.Cancelled, ""
			}
		} else {
			return auth.Denied, `{"message": "challenge timeout "}`
		}
	}

	return auth.Granted, fmt.Sprintf(`{"userinfo": %s}`, userInfoFromName(sessionInfo.username))
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

	b.isAuthenticatedCallsMu.Lock()
	defer b.isAuthenticatedCallsMu.Unlock()
	// Checks if there is a isAuthenticated call running for this session and cancels it before ending the session.
	if _, exists := b.isAuthenticatedCalls[sessionID]; exists {
		b.cancelIsAuthenticatedUnlocked(ctx, sessionID)
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
	b.cancelIsAuthenticatedUnlocked(ctx, sessionID)
}

func (b *Broker) cancelIsAuthenticatedUnlocked(_ context.Context, sessionID string) {
	b.isAuthenticatedCalls[sessionID].cancelFunc()
	delete(b.isAuthenticatedCalls, sessionID)
}

// UserPreCheck checks if the user is known to the broker.
func (b *Broker) UserPreCheck(ctx context.Context, username string) (string, error) {
	if strings.HasPrefix(username, "user-integration-pre-check") {
		return userInfoFromName(username), nil
	}
	if _, exists := exampleUsers[username]; !exists {
		return "", fmt.Errorf("user %q does not exist", username)
	}
	return userInfoFromName(username), nil
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
