package adapter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam/v2"
	"github.com/muesli/termenv"
	"github.com/skip2/go-qrcode"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/pam/internal/proto"
	pam_proto "github.com/ubuntu/authd/pam/internal/proto"
	"golang.org/x/term"
)

type nativeModel struct {
	pamMTx    pam.ModuleTransaction
	nssClient authd.NSSClient

	availableBrokers []*authd.ABResponse_BrokerInfo
	authModes        []*authd.GAMResponse_AuthenticationMode
	selectedAuthMode string
	uiLayout         *authd.UILayout

	serviceName          string
	currentStage         proto.Stage
	busy                 bool
	userSelectionAllowed bool
}

const (
	nativeCancelKey = "r"

	polkitServiceName = "polkit-1"
)

// nativeChangeStage is the internal event to notify that a stage change has happened.
type nativeChangeStage ChangeStage

// nativeStageChangeRequest is the internal event to request that a stage change.
type nativeStageChangeRequest ChangeStage

// nativeUserSelection is the internal event that an user needs to be (re)set.
type nativeUserSelection struct{}

// nativeBrokerSelection is the internal event that a broker needs to be (re)selected.
type nativeBrokerSelection struct{}

// nativeAuthSelection is used to require the user input for auth selection.
type nativeAuthSelection struct{}

// nativeChallengeRequested is used to require the user input for challenge.
type nativeChallengeRequested struct{}

// nativeAsyncOperationCompleted is a message to tell we're done with an async operation.
type nativeAsyncOperationCompleted struct{}

// nativeGoBack is a message to require to go back to previous stage.
type nativeGoBack struct{}

var errGoBack = errors.New("request to go back")
var errEmptyResponse = errors.New("empty response received")
var errNotAnInteger = errors.New("parsed value is not an integer")

// Init initializes the main model orchestrator.
func (m *nativeModel) Init() tea.Cmd {
	var err error
	m.serviceName, err = m.pamMTx.GetItem(pam.Service)
	if err != nil {
		log.Errorf(context.TODO(), "failed to get the PAM service: %v", err)
	}

	return func() tea.Msg {
		required, optional := "required", "optional"
		supportedEntries := "optional:chars,chars_password,digits,digits_password"
		requiredWithBooleans := "required:true,false"
		optionalWithBooleans := "optional:true,false"

		return supportedUILayoutsReceived{
			layouts: []*authd.UILayout{
				{
					Type:   "form",
					Label:  &required,
					Entry:  &supportedEntries,
					Wait:   &optionalWithBooleans,
					Button: &optional,
				},
				{
					Type:    "qrcode",
					Content: &required,
					Code:    &optional,
					Wait:    &requiredWithBooleans,
					Label:   &optional,
					Button:  &optional,
				},
				{
					Type:   "newpassword",
					Label:  &required,
					Entry:  &supportedEntries,
					Button: &optional,
				},
			},
		}
	}
}

func maybeSendPamError(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	var pe pam.Error
	if errors.As(err, &pe) {
		return sendEvent(pamError{status: pe, msg: err.Error()})
	}
	return sendEvent(pamError{status: pam.ErrSystem, msg: err.Error()})
}

func (m nativeModel) changeStage(stage proto.Stage) tea.Cmd {
	return sendEvent(nativeChangeStage{stage})
}

func (m nativeModel) requestStageChange(stage proto.Stage) tea.Cmd {
	return sendEvent(nativeStageChangeRequest{stage})
}

func (m nativeModel) Update(msg tea.Msg) (nativeModel, tea.Cmd) {
	log.Debugf(context.TODO(), "Native model update: %#v", msg)

	switch msg := msg.(type) {
	case nativeChangeStage:
		m.currentStage = msg.Stage

	case nativeStageChangeRequest:
		var baseCmd tea.Cmd
		if m.currentStage != msg.Stage {
			m.currentStage = msg.Stage
			baseCmd = sendEvent(ChangeStage(msg))
		}

		switch m.currentStage {
		case proto.Stage_userSelection:
			return m, tea.Sequence(baseCmd, sendEvent(nativeUserSelection{}))
		case proto.Stage_brokerSelection:
			return m, tea.Sequence(baseCmd, sendEvent(nativeBrokerSelection{}))
		case proto.Stage_authModeSelection:
			return m, tea.Sequence(baseCmd, sendEvent(nativeAuthSelection{}))
		case proto.Stage_challenge:
			return m, tea.Sequence(baseCmd, sendEvent(nativeChallengeRequested{}))
		}

	case nativeAsyncOperationCompleted:
		m.busy = false

	case nativeGoBack:
		return m.goBackCommand()

	case userRequired:
		m.userSelectionAllowed = true
		return m, m.requestStageChange(pam_proto.Stage_userSelection)

	case nativeUserSelection:
		if m.currentStage != proto.Stage_userSelection {
			return m, nil
		}
		if m.busy {
			// We may receive multiple concurrent requests, but due to the sync nature
			// of this model, we can't just accept them once we've one in progress already
			log.Debug(context.TODO(), "User selection already in progress")
			return m, nil
		}

		if cmd := maybeSendPamError(m.pamMTx.SetItem(pam.User, "")); cmd != nil {
			return m, cmd
		}

		return m.startAsyncOp(m.userSelection)

	case brokersListReceived:
		m.availableBrokers = msg.brokers

	case authModesReceived:
		m.authModes = msg.authModes

	case brokerSelectionRequired:
		if m.busy {
			// We may receive multiple concurrent requests, but due to the sync nature
			// of this model, we can't just accept them once we've one in progress already
			log.Debug(context.TODO(), "Broker selection already in progress")
			return m, nil
		}

		user, err := m.pamMTx.GetItem(pam.User)
		if err != nil {
			return m, maybeSendPamError(err)
		}
		return m.startAsyncOp(func() tea.Cmd {
			return m.maybePreCheckUser(user,
				m.requestStageChange(pam_proto.Stage_brokerSelection))
		})

	case nativeBrokerSelection:
		if m.currentStage != proto.Stage_brokerSelection {
			return m, nil
		}
		if m.busy {
			// We may receive multiple concurrent requests, but due to the sync nature
			// of this model, we can't just accept them once we've one in progress already
			log.Debug(context.TODO(), "Broker selection already in progress")
			return m, nil
		}

		if len(m.availableBrokers) < 1 {
			return m, sendEvent(pamError{
				status: pam.ErrSystem,
				msg:    "No brokers available to select",
			})
		}

		if len(m.availableBrokers) == 1 {
			return m, sendEvent(brokerSelected{brokerID: m.availableBrokers[0].Id})
		}

		return m.startAsyncOp(m.brokerSelection)

	case nativeAuthSelection:
		if m.currentStage != proto.Stage_authModeSelection {
			return m, nil
		}
		if m.busy {
			// We may receive multiple concurrent requests, but due to the sync nature
			// of this model, we can't just accept them once we've one in progress already
			log.Debug(context.TODO(), "Authentication selection already in progress")
			return m, nil
		}
		if m.selectedAuthMode != "" {
			return m, nil
		}
		if len(m.authModes) < 1 {
			return m, sendEvent(pamError{
				status: pam.ErrSystem,
				msg:    "Can't authenticate without authentication modes",
			})
		}

		if len(m.authModes) == 1 {
			return m, sendEvent(authModeSelected{id: m.authModes[0].Id})
		}

		return m.startAsyncOp(m.authModeSelection)

	case authModeSelected:
		m.selectedAuthMode = msg.id

	case UILayoutReceived:
		m.uiLayout = msg.layout

	case startAuthentication:
		return m, m.requestStageChange(pam_proto.Stage_challenge)

	case nativeChallengeRequested:
		if m.currentStage != pam_proto.Stage_challenge {
			return m, nil
		}
		return m, m.startChallenge()

	case newPasswordCheckResult:
		if msg.msg != "" {
			if cmd := maybeSendPamError(m.sendError(msg.msg)); cmd != nil {
				return m, cmd
			}
			return m, m.newPasswordChallenge(nil)
		}
		return m, m.newPasswordChallenge(&msg.challenge)

	case isAuthenticatedResultReceived:
		access := msg.access
		authMsg, err := dataToMsg(msg.msg)
		if cmd := maybeSendPamError(err); cmd != nil {
			return m, cmd
		}

		switch access {
		case brokers.AuthGranted:
			return m, maybeSendPamError(m.sendInfo(authMsg))
		case brokers.AuthNext:
			m.uiLayout = nil
			return m, maybeSendPamError(m.sendInfo(authMsg))
		case brokers.AuthRetry:
			return m, maybeSendPamError(m.sendError(authMsg))
		case brokers.AuthDenied:
			// This is handled by the main authentication model
			return m, nil
		case brokers.AuthCancelled:
			return m, sendEvent(isAuthenticatedCancelled{})
		default:
			return m, maybeSendPamError(m.sendError("Access %q is not valid", access))
		}

	case isAuthenticatedCancelled:
		return m.goBackCommand()
	}

	return m, nil
}

func (m nativeModel) checkForPromptReplyValidity(reply string) error {
	switch reply {
	case nativeCancelKey:
		if m.canGoBack() {
			return errGoBack
		}
	case "", "\n":
		return errEmptyResponse
	}
	return nil
}

func (m nativeModel) promptForInput(style pam.Style, prompt string) (string, error) {
	resp, err := m.pamMTx.StartStringConvf(style, "%s: ", prompt)
	if err != nil {
		return "", err
	}
	return resp.Response(), m.checkForPromptReplyValidity(resp.Response())
}

func (m nativeModel) promptForNumericInput(style pam.Style, prompt string) (int, error) {
	out, err := m.promptForInput(style, prompt)
	if err != nil {
		return -1, err
	}

	intOut, err := strconv.Atoi(out)
	if err != nil {
		return intOut, fmt.Errorf("%w: %w", errNotAnInteger, err)
	}

	return intOut, err
}

func (m nativeModel) promptForNumericInputUntilValid(style pam.Style, prompt string) (int, error) {
	value, err := m.promptForNumericInput(style, prompt)
	if !errors.Is(err, errNotAnInteger) {
		return value, err
	}

	err = m.sendError("Provided input can't be parsed as integer value")
	if err != nil {
		return -1, err
	}

	return m.promptForNumericInputUntilValid(style, prompt)
}

func (m nativeModel) promptForNumericInputAsString(style pam.Style, prompt string) (string, error) {
	input, err := m.promptForNumericInputUntilValid(style, prompt)
	return fmt.Sprint(input), err
}

func (m nativeModel) sendError(errorMsg string, args ...any) error {
	if errorMsg == "" {
		return nil
	}
	_, err := m.pamMTx.StartStringConvf(pam.ErrorMsg, errorMsg, args...)
	return err
}

func (m nativeModel) sendInfo(infoMsg string, args ...any) error {
	if infoMsg == "" {
		return nil
	}
	_, err := m.pamMTx.StartStringConvf(pam.TextInfo, infoMsg, args...)
	return err
}

type choicePair struct {
	id    string
	label string
}

func (m nativeModel) promptForChoice(title string, choices []choicePair, prompt string) (string, error) {
	if m.canGoBack() {
		title = fmt.Sprintf("%s (use '%s' to go back)", title, nativeCancelKey)
	}

	msg := fmt.Sprintf("== %s ==\n", title)
	for i, choice := range choices {
		msg += fmt.Sprintf("%d - %s\n", i+1, choice.label)
	}
	msg += prompt

	for {
		idx, err := m.promptForNumericInputUntilValid(pam.PromptEchoOn, msg)
		if err != nil {
			return "", err
		}
		// TODO: Maybe add support for default selection...

		if idx < 1 || idx > len(choices) {
			if err := m.sendError("Invalid entry. Try again or input '%s'.", nativeCancelKey); err != nil {
				return "", err
			}
			continue
		}

		return choices[idx-1].id, nil
	}
}

func (m nativeModel) startAsyncOp(cmd func() tea.Cmd) (nativeModel, tea.Cmd) {
	m.busy = true
	return m, func() tea.Msg {
		ret := cmd()
		return tea.Sequence(
			sendEvent(nativeAsyncOperationCompleted{}),
			ret,
		)()
	}
}

func (m nativeModel) userSelection() tea.Cmd {
	user, err := m.promptForInput(pam.PromptEchoOn, "Username")
	if errors.Is(err, errEmptyResponse) {
		return sendEvent(nativeUserSelection{})
	}
	if err != nil {
		return maybeSendPamError(err)
	}

	return m.maybePreCheckUser(user, sendEvent(userSelected{user}))
}

func (m nativeModel) maybePreCheckUser(user string, nextCmd tea.Cmd) tea.Cmd {
	if m.nssClient == nil {
		return nextCmd
	}

	// When the NSS client is defined (i.e. under SSH for now) we want also
	// repeat the user pre-check, to ensure that the user is handled by at least
	// one broker, or we may end up leaking such infos.
	// We don't care about the content, we only care if the user is known by some broker.
	_, err := m.nssClient.GetPasswdByName(context.TODO(), &authd.GetPasswdByNameRequest{
		Name:           user,
		ShouldPreCheck: true,
	})
	if err != nil {
		log.Infof(context.TODO(), "can't get user info for %q: %v", user, err)
		return sendEvent(brokerSelected{brokerID: brokers.LocalBrokerName})
	}
	return nextCmd
}

func (m nativeModel) brokerSelection() tea.Cmd {
	var choices []choicePair
	for _, b := range m.availableBrokers {
		choices = append(choices, choicePair{id: b.Id, label: b.Name})
	}

	id, err := m.promptForChoice("Broker selection", choices, "Select broker")
	if errors.Is(err, errGoBack) {
		return sendEvent(nativeGoBack{})
	}
	if err != nil {
		return sendEvent(pamError{
			status: pam.ErrSystem,
			msg:    fmt.Sprintf("broker selection error: %v", err),
		})
	}
	return sendEvent(brokerSelected{brokerID: id})
}

func (m nativeModel) authModeSelection() tea.Cmd {
	var choices []choicePair
	for _, am := range m.authModes {
		choices = append(choices, choicePair{id: am.Id, label: am.Label})
	}

	id, err := m.promptForChoice("Authentication mode selection", choices,
		"Select authentication mode")
	if errors.Is(err, errGoBack) {
		return sendEvent(nativeGoBack{})
	}
	if errors.Is(err, errEmptyResponse) {
		return m.requestStageChange(pam_proto.Stage_challenge)
	}
	if err != nil {
		return sendEvent(pamError{
			status: pam.ErrSystem,
			msg:    fmt.Sprintf("broker selection error: %v", err),
		})
	}

	return sendEvent(authModeSelected{id: id})
}

func (m nativeModel) startChallenge() tea.Cmd {
	if m.uiLayout == nil {
		return sendEvent(pamError{
			status: pam.ErrSystem,
			msg:    "Can't authenticate without ui layout selected",
		})
	}

	hasWait := m.uiLayout.GetWait() == "true"

	switch m.uiLayout.Type {
	case "form":
		return m.handleFormChallenge(hasWait)

	case "qrcode":
		if !hasWait {
			return sendEvent(pamError{
				status: pam.ErrSystem,
				msg:    "Can't handle qrcode without waiting",
			})
		}
		return m.handleQrCode()

	case "newpassword":
		return m.handleNewPassword()

	default:
		return sendEvent(pamError{
			status: pam.ErrSystem,
			msg:    fmt.Sprintf("Unknown layout type: %q", m.uiLayout.Type),
		})
	}
}

func (m nativeModel) handleFormChallenge(hasWait bool) tea.Cmd {
	if buttonLabel := m.uiLayout.GetButton(); buttonLabel != "" {
		authMode := "selected authentication mode"
		authModeIdx := slices.IndexFunc(m.authModes, func(mode *authd.GAMResponse_AuthenticationMode) bool {
			return mode.Id == m.selectedAuthMode
		})
		if authModeIdx > -1 {
			authMode = m.authModes[authModeIdx].Label
		}
		choices := []choicePair{
			{id: "continue", label: fmt.Sprintf("Proceed with %s", authMode)},
		}
		if buttonLabel := m.uiLayout.GetButton(); buttonLabel != "" {
			choices = append(choices, choicePair{id: "button", label: buttonLabel})
		}

		id, err := m.promptForChoice(authMode, choices, "Select action")
		if errors.Is(err, errGoBack) {
			return sendEvent(nativeGoBack{})
		}
		if errors.Is(err, errEmptyResponse) {
			return sendEvent(nativeChallengeRequested{})
		}
		if err != nil {
			return maybeSendPamError(err)
		}
		if id == "button" {
			return sendEvent(reselectAuthMode{})
		}
	}

	var prompt string
	if m.uiLayout.Label != nil {
		prompt, _ = strings.CutSuffix(*m.uiLayout.Label, ":")
	}

	if prompt == "" {
		switch m.uiLayout.GetEntry() {
		case "digits":
			fallthrough
		case "digits_password":
			prompt = "PIN"
		case "chars":
			prompt = "Value"
		case "chars_password":
			prompt = "Password"
		}
	}

	instructions := "Insert '%[1]s' to cancel the request and go back"
	if hasWait {
		// Duplicating some contents here, as it will be better for translators once we've them
		instructions = "Leave the input field empty to wait for other authentication method " +
			"or insert '%[1]s' to go back"
		if m.uiLayout.GetEntry() == "" {
			instructions = "Leave the input field empty to wait for the authentication method " +
				"or insert '%[1]s' to go back"
		}
	}

	if cmd := maybeSendPamError(m.sendInfo(instructions, nativeCancelKey)); cmd != nil {
		return cmd
	}

	challenge, err := m.promptForChallenge(prompt)
	if errors.Is(err, errGoBack) {
		return sendEvent(nativeGoBack{})
	}
	if errors.Is(err, errEmptyResponse) {
		if hasWait {
			return sendAuthWaitCommand()
		}
		err = nil
	}
	if err != nil {
		return maybeSendPamError(err)
	}

	return sendEvent(isAuthenticatedRequested{
		item: &authd.IARequest_AuthenticationData_Challenge{Challenge: challenge},
	})
}

func (m nativeModel) promptForChallenge(prompt string) (string, error) {
	switch m.uiLayout.GetEntry() {
	case "chars", "":
		return m.promptForInput(pam.PromptEchoOn, prompt)
	case "chars_password":
		return m.promptForInput(pam.PromptEchoOff, prompt)
	case "digits":
		return m.promptForNumericInputAsString(pam.PromptEchoOn, prompt)
	case "digits_password":
		return m.promptForNumericInputAsString(pam.PromptEchoOff, prompt)
	default:
		return "", fmt.Errorf("Unhandled entry %q", m.uiLayout.GetEntry())
	}
}

func (m nativeModel) getPamTty() (*os.File, error) {
	pamTty, err := m.pamMTx.GetItem(pam.Tty)
	if err != nil {
		return nil, err
	}

	if pamTty == "" {
		return nil, errors.New("no PAM_TTY value set")
	}

	f, err := os.OpenFile(pamTty, os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (m nativeModel) isTerminalTty() bool {
	tty, err := m.getPamTty()
	// We check the fd could be passed to x/term to decide if we should fallback to stdin
	if err == nil {
		defer tty.Close()
		if tty.Fd() > math.MaxInt {
			err = fmt.Errorf("unexpected large PAM TTY fd: %d", tty.Fd())
		}
	}
	if err != nil {
		log.Debugf(context.TODO(), "Failed to open PAM TTY: %s", err)
		tty = os.Stdin
	}

	return term.IsTerminal(int(tty.Fd()))
}

func (m nativeModel) renderQrCode(qrCode *qrcode.QRCode) (qr string) {
	defer func() { qr = strings.TrimRight(qr, "\n") }()

	if os.Getenv("XDG_SESSION_TYPE") == "tty" {
		return qrCode.ToString(false)
	}

	switch termenv.DefaultOutput().Profile {
	case termenv.ANSI, termenv.Ascii:
		return qrCode.ToString(false)
	default:
		return qrCode.ToSmallString(false)
	}
}

func (m nativeModel) handleQrCode() tea.Cmd {
	qrCode, err := qrcode.New(m.uiLayout.GetContent(), qrcode.Medium)
	if err != nil {
		return sendEvent(pamError{
			status: pam.ErrSystem,
			msg:    fmt.Sprintf("Can't generate qrcode: %v", err),
		})
	}

	var qrcodeView []string
	qrcodeView = append(qrcodeView, m.uiLayout.GetLabel())

	var firstQrCodeLine string
	if m.isQrcodeRenderingSupported() {
		qrcode := m.renderQrCode(qrCode)
		qrcodeView = append(qrcodeView, qrcode)
		firstQrCodeLine = strings.SplitN(qrcode, "\n", 2)[0]
	}
	if firstQrCodeLine == "" {
		firstQrCodeLine = m.uiLayout.GetContent()
	}

	centeredContent := centerString(m.uiLayout.GetContent(), firstQrCodeLine)
	qrcodeView = append(qrcodeView, centeredContent)

	if code := m.uiLayout.GetCode(); code != "" {
		qrcodeView = append(qrcodeView, centerString(code, firstQrCodeLine))
	}

	// Ass some extra vertical space to improve readability
	qrcodeView = append(qrcodeView, " ")

	if cmd := maybeSendPamError(m.sendInfo(strings.Join(qrcodeView, "\n"))); cmd != nil {
		return cmd
	}

	choices := []choicePair{
		{id: "wait", label: "Wait for the QR code scan result"},
	}
	if buttonLabel := m.uiLayout.GetButton(); buttonLabel != "" {
		choices = append(choices, choicePair{id: "button", label: buttonLabel})
	}

	id, err := m.promptForChoice("Qr Code authentication", choices, "Select action")
	if errors.Is(err, errGoBack) {
		return sendEvent(nativeGoBack{})
	}
	if errors.Is(err, errEmptyResponse) {
		return sendAuthWaitCommand()
	}
	if err != nil {
		return maybeSendPamError(err)
	}

	switch id {
	case "button":
		return sendEvent(reselectAuthMode{})
	case "wait":
		return sendAuthWaitCommand()
	default:
		return nil
	}
}

func (m nativeModel) isQrcodeRenderingSupported() bool {
	switch m.serviceName {
	case polkitServiceName:
		return false
	default:
		if IsSSHSession(m.pamMTx) {
			return false
		}
		return m.isTerminalTty()
	}
}

func centerString(s string, reference string) string {
	sizeDiff := len([]rune(reference)) - len(s)
	if sizeDiff <= 0 {
		return s
	}

	// We put padding in both sides, so that it's respected also by non-terminal UIs
	padding := strings.Repeat(" ", sizeDiff/2)
	return padding + s + padding
}

func (m nativeModel) handleNewPassword() tea.Cmd {
	if buttonLabel := m.uiLayout.GetButton(); buttonLabel != "" {
		choices := []choicePair{
			{id: "continue", label: "Proceed with password update"},
		}
		if buttonLabel := m.uiLayout.GetButton(); buttonLabel != "" {
			choices = append(choices, choicePair{id: "button", label: buttonLabel})
		}

		id, err := m.promptForChoice("Password Update", choices, "Select action")
		if errors.Is(err, errGoBack) {
			return sendEvent(nativeGoBack{})
		}
		if errors.Is(err, errEmptyResponse) {
			return sendEvent(nativeChallengeRequested{})
		}
		if err != nil {
			return maybeSendPamError(err)
		}
		if id == "button" {
			return sendEvent(isAuthenticatedRequested{
				item: &authd.IARequest_AuthenticationData_Skip{Skip: "true"},
			})
		}
	}

	return m.newPasswordChallenge(nil)
}

func (m nativeModel) newPasswordChallenge(previousChallenge *string) tea.Cmd {
	if previousChallenge == nil {
		if cmd := maybeSendPamError(m.sendInfo("Insert '%[1]s' to cancel the request and go back",
			nativeCancelKey)); cmd != nil {
			return cmd
		}
	} else {
		if cmd := maybeSendPamError(m.sendInfo("Repeat the previously inserted password or insert '%[1]s' to cancel the request and go back",
			nativeCancelKey)); cmd != nil {
			return cmd
		}
	}

	challenge, err := m.promptForChallenge(m.uiLayout.GetLabel())
	if errors.Is(err, errGoBack) {
		return sendEvent(nativeGoBack{})
	}
	if err != nil && !errors.Is(err, errEmptyResponse) {
		return maybeSendPamError(err)
	}

	if previousChallenge == nil {
		return sendEvent(newPasswordCheck{challenge: challenge})
	}
	if challenge != *previousChallenge {
		err := m.sendError("Password entries don't match")
		if err != nil {
			return maybeSendPamError(err)
		}
		return m.newPasswordChallenge(nil)
	}
	return sendEvent(isAuthenticatedRequested{
		item: &authd.IARequest_AuthenticationData_Challenge{Challenge: challenge},
	})
}

func (m nativeModel) goBackCommand() (nativeModel, tea.Cmd) {
	if m.currentStage >= proto.Stage_challenge && m.uiLayout != nil {
		m.uiLayout = nil
		return m, sendEvent(isAuthenticatedCancelled{})
	}
	if m.currentStage >= proto.Stage_authModeSelection {
		m.selectedAuthMode = ""
	}
	if m.currentStage == proto.Stage_authModeSelection {
		m.authModes = nil
	}

	return m, func() tea.Cmd {
		if !m.canGoBack() {
			return nil
		}
		return m.requestStageChange(m.previousStage())
	}()
}

func (m nativeModel) canGoBack() bool {
	if m.userSelectionAllowed {
		return m.currentStage > proto.Stage_userSelection
	}
	return m.previousStage() > proto.Stage_userSelection
}

func (m nativeModel) previousStage() pam_proto.Stage {
	if m.currentStage > proto.Stage_authModeSelection && len(m.authModes) > 1 {
		return proto.Stage_authModeSelection
	}
	if m.currentStage > proto.Stage_brokerSelection && len(m.availableBrokers) > 1 {
		return proto.Stage_brokerSelection
	}
	return proto.Stage_userSelection
}

func sendAuthWaitCommand() tea.Cmd {
	return sendEvent(isAuthenticatedRequested{
		item: &authd.IARequest_AuthenticationData_Wait{Wait: "true"},
	})
}
