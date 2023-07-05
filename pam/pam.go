package main

/*
#cgo LDFLAGS: -lpam -fPIC
#include <security/pam_appl.h>
#include <security/pam_ext.h>
#include <stdlib.h>
#include <string.h>

char *string_from_argv(int i, char **argv);
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/log"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	errGoBack error = errors.New("needs go back")
)

const (
	maxChallengeRetries = 3
)

//go:generate sh -c "go build -ldflags='-extldflags -Wl,-soname,pam_authd.so' -buildmode=c-shared -o pam_authd.so"

/*
	Add to /etc/pam.d/common-auth
	auth    [success=3 default=die ignore=ignore]   pam_authd.so
*/

//export pam_sm_authenticate
func pam_sm_authenticate(pamh *C.pam_handle_t, flags, argc C.int, argv **C.char) C.int {
	// Initialize localization
	// TODO

	// Attach logger and info handler.
	// TODO
	client, err := newClient(argc, argv)
	if err != nil {
		log.Debugf(context.TODO(), "%s", err)
		return C.PAM_IGNORE
	}

	// Get current user for broker.
	user, err := getUser(pamh, "login: ")
	if err != nil {
		log.Errorf(context.TODO(), "Can't get user: %v", err)
		return C.PAM_AUTH_ERR
	}

	brokersInfo, err := client.AvailableBrokers(context.TODO(), &authd.ABRequest{
		UserName: &user,
	})
	if err != nil {
		log.Errorf(context.TODO(), "Could not get current available brokers: %v", err)
		return C.PAM_IGNORE
	}

	type Stage int
	const (
		StageBrokerSelection Stage = iota
		StageAuthenticationMode
		StageChallenge
	)

	stage := StageBrokerSelection

	var brokerName, sessionID, encryptionKey string
	brokerID := brokersInfo.GetPreviousBroker()
	// ensure previous broker for this user still exists
	if brokerID != "" {
		var stillAlive bool
		for _, b := range brokersInfo.GetBrokersInfos() {
			if b.Id != brokerID {
				continue
			}
			stillAlive = true
			brokerName = b.Name
		}
		if !stillAlive {
			brokerID = ""
		}
	}

	var currentAuthModeName string
	var availableAuthModes []*authd.SBResponse_AuthenticationMode
	var uiLayout *authd.UILayout

	for {
		var challengeRetry int

		switch stage {
		case StageBrokerSelection:
			// Broker selection and escape
			if brokerID == "" {
				brokerID, brokerName, err = selectBrokerInteractive(brokersInfo.GetBrokersInfos())
				if err != nil {
					// Do not show error message if we only wanted to reset everything from the start, including user name.
					if !errors.Is(err, errGoBack) {
						log.Errorf(context.TODO(), "could not get selected broker: %v", err)
					}
					return C.PAM_SYSTEM_ERR
				}
			}
			if brokerID == "local" {
				return C.PAM_IGNORE
			}
			sessionID, availableAuthModes, encryptionKey, err = startBrokerSession(client, brokerID, user)
			if err != nil {
				log.Errorf(context.TODO(), "can't select broker %q: %v", brokerName, err)
				return C.PAM_SYSTEM_ERR
			}

			// Autoselect first one.
			currentAuthModeName = availableAuthModes[0].Name
			stage = StageAuthenticationMode

		case StageAuthenticationMode:
			if currentAuthModeName == "" {
				currentAuthModeName, err = selectAuthenticationModeInteractive(availableAuthModes)
				// Return one level up, to broker selection.
				if errors.Is(err, errGoBack) {
					brokerID = ""
					stage = StageBrokerSelection
					continue
				}
				if err != nil {
					log.Errorf(context.TODO(), "can't select interactively authentication mode: %v", err)
					return C.PAM_SYSTEM_ERR
				}
			}

			// Ask broker for UI specific information.
			samReq := &authd.SAMRequest{
				SessionId:              sessionID,
				AuthenticationModeName: currentAuthModeName,
			}
			uiInfo, err := client.SelectAuthenticationMode(context.TODO(), samReq)
			if err != nil {
				log.Errorf(context.TODO(), "can't select authentication mode: %v", err)
				return C.PAM_SYSTEM_ERR
			}

			if uiInfo.UiLayoutInfo == nil {
				log.Errorf(context.TODO(), "invalid empty UI Layout information from broker")
				return C.PAM_SYSTEM_ERR
			}

			uiLayout = uiInfo.UiLayoutInfo
			stage = StageChallenge

		case StageChallenge:
			var iaResp *authd.IAResponse
			var err error
			// TODO: handle context cancellation to return to previous authentication method selection.

			switch uiLayout.Type {
			case "form":
				iaResp, err = formChallenge(client, sessionID, encryptionKey, uiLayout)

			case "qrcode":
				iaResp, err = qrcodeChallenge(client, sessionID, encryptionKey, uiLayout)
			}

			// Go back to authentication selection.
			if errors.Is(err, errGoBack) {
				stage = StageAuthenticationMode
				continue
			}

			// Validate answer contains something
			if err == nil && iaResp == nil {
				err = errors.New("empty reponse")
			}
			if err != nil {
				log.Errorf(context.TODO(), "can't check for authorization: %v", err)
				return C.PAM_SYSTEM_ERR
			}

			// Check if authorized
			switch strings.ToLower(iaResp.Access) {
			case "denied":
				fmt.Println("Access Denied")
				challengeRetry++
				if challengeRetry < maxChallengeRetries {
					fmt.Println("Retrying")
					continue
				}
				return C.PAM_AUTH_ERR
			case "allowed":
				fmt.Println("Welcome")
				for k, v := range iaResp.UserInfo {
					fmt.Printf("Key: %s, Value: %s\n", k, v)
				}
				return C.PAM_SUCCESS
			default:
				// Invalid response
				log.Errorf(context.TODO(), "Invalid Reponse: %v", iaResp.Access)
				return C.PAM_SYSTEM_ERR
			}
		}
	}
}

//export pam_sm_setcred
func pam_sm_setcred(pamh *C.pam_handle_t, flags, argc C.int, argv **C.char) C.int {
	return C.PAM_IGNORE
}

//export pam_sm_open_session
func pam_sm_open_session(pamh *C.pam_handle_t, flags, argc C.int, argv **C.char) C.int {
	return C.PAM_SUCCESS
}

//export pam_sm_close_session
func pam_sm_close_session(pamh *C.pam_handle_t, flags, argc C.int, argv **C.char) C.int {
	return C.PAM_SUCCESS
}

// selectBroker allows interactive broker selection.
// Only one choice will be returned immediately.
func selectBrokerInteractive(brokersInfo []*authd.ABResponse_BrokerInfo) (brokerID, brokerName string, err error) {
	if len(brokersInfo) < 1 {
		return "", "", errors.New("no broker found")
	}

	// Default choice for one possibility.
	if len(brokersInfo) == 1 {
		return brokersInfo[0].GetId(), brokersInfo[0].GetName(), nil
	}

	var choices []string
	var ids []string
	for i, b := range brokersInfo {
		choices = append(choices, fmt.Sprintf("%d - %s, %s", i+1, b.GetName(), b.GetBrandIcon()))
		ids = append(ids, b.GetId())
	}

	i, err := promptForInt("= Broker selection =", choices, "Select broker ('r' to cancel): ")
	if err != nil {
		return "", "", fmt.Errorf("broker selection error: %w", err)
	}

	return ids[i], brokersInfo[i].GetName(), nil
}

// startBrokerSession returns the sessionID and available authentication modes after marking a broker as current.
func startBrokerSession(client authd.PAMClient, brokerID, username string) (sessionID string, authModes []*authd.SBResponse_AuthenticationMode, encryptionKey string, err error) {
	// Start a transaction for this user with the broker.
	lang := "C"
	for _, e := range []string{"LANG", "LC_MESSAGES", "LC_ALL"} {
		l := os.Getenv(e)
		if l != "" {
			lang = l
		}
	}
	lang = strings.TrimSuffix(lang, ".UTF-8")

	required, optional := "required", "optional"
	supportedEntries := "optional:chars,chars_password"
	waitRequired := "required:true,false"
	waitOptional := "optional:true,false"
	sbReq := &authd.SBRequest{
		BrokerId: brokerID,
		Username: username,
		Lang:     lang,
		SupportedUiLayouts: []*authd.UILayout{
			{
				Type:  "form",
				Label: &required,
				Entry: &supportedEntries,
				Wait:  &waitOptional,
			},
			{
				Type:    "qrcode",
				Content: &required,
				Wait:    &waitRequired,
				Label:   &optional,
			},
		},
	}

	sbResp, err := client.SelectBroker(context.TODO(), sbReq)
	if err != nil {
		return "", nil, "", fmt.Errorf("can't get authentication mode: %v", err)
	}

	sessionID = sbResp.GetSessionId()
	if sessionID == "" {
		return "", nil, "", errors.New("no session ID returned by broker")
	}
	encryptionKey = sbResp.GetEncryptionKey()
	if encryptionKey == "" {
		return "", nil, "", errors.New("no encryption key returned by broker")
	}
	availableAuthModes := sbResp.GetAuthenticationModes()
	if len(availableAuthModes) == 0 {
		return "", nil, "", errors.New("no supported authentication mode available for this broker")
	}

	return sessionID, availableAuthModes, encryptionKey, nil
}

// selectAuthenticationModeInteractive allows interactive authentication mode selection.
// Only one choice will be returned immediately.
func selectAuthenticationModeInteractive(authModes []*authd.SBResponse_AuthenticationMode) (name string, err error) {
	if len(authModes) < 1 {
		return "", errors.New("no auhentication mode supported")
	}

	// Default choice for one possibility.
	if len(authModes) == 1 {
		return authModes[0].GetName(), nil
	}

	var choices []string
	var ids []string
	for i, m := range authModes {
		choices = append(choices, fmt.Sprintf("%d - %s\n", i+1, m.Label))
		ids = append(ids, m.GetName())
	}

	i, err := promptForInt("= Authentication mode =", choices, "Select authentication mode ('r' to cancel): ")
	if err != nil {
		return "", fmt.Errorf("authentication mode selection error: %w", err)
	}

	return ids[i], nil
}

func promptForInt(title string, choices []string, prompt string) (r int, err error) {
	fmt.Println(title)

	for {
		fmt.Println()
		for i, msg := range choices {
			fmt.Printf("%d - %s\n", i+1, msg)
		}

		fmt.Print(prompt)
		var r string
		if _, err = fmt.Scanln(&r); err != nil {
			return 0, fmt.Errorf("error while reading stdin: %v", err)
		}
		if r == "r" {
			return 0, errGoBack
		}
		if r == "" {
			r = "1"
		}

		choice, err := strconv.Atoi(r)
		if err != nil || choice < 1 || choice > len(choices) {
			log.Errorf(context.TODO(), "Invalid entry. Try again or type 'r'.")
			continue
		}

		return choice - 1, nil
	}
}

func formChallenge(client authd.PAMClient, sessionID, encryptionKey string, uiLayout *authd.UILayout) (iaResp *authd.IAResponse, err error) {
	prompt := uiLayout.GetLabel()
	if !strings.HasSuffix(prompt, " ") {
		prompt = fmt.Sprintf("%s ", prompt)
	}
	fmt.Printf("%s ('r' to cancel)", prompt)

	type result struct {
		iaResp *authd.IAResponse
		err    error
	}
	results := make(chan result)

	waitCtx, cancelWait := context.WithCancel(context.Background())
	defer cancelWait()
	termCtx, cancelTerm := context.WithCancel(context.Background())
	defer cancelTerm()

	if uiLayout.GetWait() == "true" {
		// We can ask for an immediate authorization without challenge
		go func() {
			var err error
			iaResp, err := client.IsAuthorized(waitCtx, &authd.IARequest{
				SessionId:          sessionID,
				AuthenticationData: "",
			})

			// No more processing if entry has been filed.
			select {
			case <-waitCtx.Done():
				return
			default:
			}

			cancelTerm()

			results <- result{
				iaResp: iaResp,
				err:    err,
			}
		}()
	}

	if uiLayout.GetEntry() == "chars" || uiLayout.GetEntry() == "chars_password" {
		go func() {
			out, err := readPasswordWithContext(int(os.Stdin.Fd()), termCtx, uiLayout.GetEntry() == "chars_password")

			// No more processing if wait IsAuthorized has been answered.
			select {
			case <-termCtx.Done():
				return
			default:
				// Immediately cancel wait goroutine, we won't care about its result.
				cancelWait()
			}

			if err != nil {
				results <- result{
					iaResp: nil,
					err:    err,
				}
			}

			authData := "{}"
			challenge := string(out)
			if challenge != "" {
				// TODO: encrypt with encryptionKey
				authData = fmt.Sprintf(`{"challenge": "%s"}`, challenge)
			}

			// Validate challenge with Broker
			iaReq := &authd.IARequest{
				SessionId:          sessionID,
				AuthenticationData: authData,
			}
			iaResp, err := client.IsAuthorized(context.TODO(), iaReq)
			results <- result{
				iaResp: iaResp,
				err:    err,
			}
		}()
	} else {
		fmt.Print("\n")
		// TODO: input handling to escape
	}

	r := <-results
	if r.err != nil {
		return nil, err
	}

	return r.iaResp, nil
}

func qrcodeChallenge(client authd.PAMClient, sessionID, encryptionKey string, uiLayout *authd.UILayout) (iaResp *authd.IAResponse, err error) {
	l := uiLayout.GetLabel()
	if l != "" {
		fmt.Println(l)
	}
	qrCode, err := qrcode.New(uiLayout.GetContent(), qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("can't generate QR code: %v", err)
	}
	asciiQR := qrCode.ToSmallString(false)
	fmt.Println(asciiQR)

	iaReq := &authd.IARequest{
		SessionId:          sessionID,
		AuthenticationData: "",
	}
	iaResp, err = client.IsAuthorized(context.TODO(), iaReq)
	if err != nil {
		return nil, err
	}

	return iaResp, nil
}

func readPasswordWithContext(fd int, ctx context.Context, password bool) ([]byte, error) {
	const ioctlReadTermios = unix.TCGETS
	const ioctlWriteTermios = unix.TCSETS

	termios, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	nonblocking := false
	if err != nil {
		return nil, err
	}
	newState := *termios
	if password {
		newState.Lflag &^= unix.ECHO
	}
	newState.Lflag |= unix.ICANON | unix.ISIG
	newState.Iflag |= unix.ICRNL

	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &newState); err != nil {
		return nil, err
	}
	defer func() {
		if nonblocking {
			unix.SetNonblock(fd, false)
		}
		unix.IoctlSetTermios(fd, ioctlWriteTermios, termios)
	}()

	// Set nonblocking IO
	if err := unix.SetNonblock(fd, true); err != nil {
		return nil, err
	}
	nonblocking = true

	var ret []byte
	var buf [1]byte
	for {
		if ctx.Err() != nil {
			return ret, ctx.Err()
		}
		n, err := unix.Read(fd, buf[:])
		if err != nil {
			// Check for nonblocking error
			if serr, ok := err.(syscall.Errno); ok {
				if serr == 11 {
					// Add (hopefully not noticable) latency to prevent CPU hogging
					time.Sleep(50 * time.Millisecond)
					continue
				}
			}
			return ret, err
		}
		if n > 0 {
			switch buf[0] {
			case '\b':
				if len(ret) > 0 {
					ret = ret[:len(ret)-1]
				}
			case '\n':
				return ret, nil
			case 'r':
				// Only return if r is the single character entered.
				if len(ret) > 0 {
					return nil, errGoBack
				}
			default:
				ret = append(ret, buf[0])
			}
			continue
		}
	}
}

// newClient returns a new GRPC client ready to emit requests
func newClient(argc C.int, argv **C.char) (authd.PAMClient, error) {
	conn, err := grpc.Dial("unix://"+getSocketPath(argc, argv), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("could not connect to authd: %v", err)
	}
	return authd.NewPAMClient(conn), nil
}

// getSocketPath returns the socket path to connect to which can be overriden manually.
func getSocketPath(argc C.int, argv **C.char) string {
	socketPath := consts.DefaultSocketPath
	for _, arg := range sliceFromArgv(argc, argv) {
		opt, optarg, _ := strings.Cut(arg, "=")
		switch opt {
		case "socket":
			socketPath = optarg
		default:
		}
	}
	return socketPath
}

func main() {

}
