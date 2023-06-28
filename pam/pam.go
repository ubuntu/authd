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

	socketPath := consts.DefaultSocketPath
	for _, arg := range sliceFromArgv(argc, argv) {
		opt, optarg, _ := strings.Cut(arg, "=")
		switch opt {
		case "socket":
			socketPath = optarg
		default:

		}
	}

	conn, err := grpc.Dial("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Debugf(context.TODO(), "Could not connect to authd: %v", err)
		return C.PAM_IGNORE
	}
	client := authd.NewPAMClient(conn)
	info, err := client.GetBrokersInfo(context.TODO(), &authd.Empty{})
	if err != nil {
		log.Error(context.TODO(), err)
		return C.PAM_IGNORE
	}

	if len(info.BrokersInfos) == 0 {
		return C.PAM_IGNORE
	}

	brokerName := os.Getenv("AUTHD_CURRENT_BROKER")
	if brokerName == "" {
		brokerName = selectBroker(info.GetBrokersInfos())
	}
	if brokerName == "" {
		return C.PAM_IGNORE
	}

	// Inform selected broker with the user name info.
	user, err := getUser(pamh, "login: ")
	if err != nil {
		fmt.Printf("Error: can't get user: %v", err)
		return C.PAM_AUTH_ERR
	}

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
	gamReq := &authd.GetAuthenticationModesRequest{
		Broker:   brokerName,
		Username: user,
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
	authMode, err := client.GetAuthenticationModes(context.TODO(), gamReq)
	if err != nil {
		fmt.Printf("Error: can't get authentication mode: %v", err)
		return C.PAM_AUTH_ERR
	}
	sessionID := authMode.SessionId

	authModeName := selectAuthenticationMode(authMode.GetAuthenticationModes())
	if authModeName == "" {
		return C.PAM_AUTH_ERR
	}

	// Ask broker for UI specific information.
	samReq := &authd.SAMRequest{
		SessionId:              sessionID,
		AuthenticationModeName: authModeName,
	}
	uiInfo, err := client.SelectAuthenticationMode(context.TODO(), samReq)
	if err != nil {
		fmt.Printf("Error: can't select authentication mode: %v", err)
		return C.PAM_AUTH_ERR
	}

	var iaResp *authd.IAResponse

	// Show UI with additional info and select different mode of action
	switch uiLayoutInfo := uiInfo.GetUiLayoutInfo(); uiLayoutInfo.Type {
	case "form":
		prompt := uiLayoutInfo.GetLabel()
		if prompt == "" {
			// TODO error
			return C.PAM_SYSTEM_ERR
		}
		if !strings.HasSuffix(prompt, " ") {
			prompt = fmt.Sprintf("%s ", prompt)
		}
		fmt.Print(prompt)

		type result struct {
			iaResp *authd.IAResponse
			err    error
		}
		results := make(chan result)

		waitCtx, cancelWait := context.WithCancel(context.Background())
		defer cancelWait()
		termCtx, cancelTerm := context.WithCancel(context.Background())
		defer cancelTerm()

		if uiLayoutInfo.GetWait() == "true" {
			// We can ask for an immediate authorization without challenge
			go func() {
				var err error
				iaResp, err := client.IsAuthorized(waitCtx, &authd.IARequest{
					SessionId:          sessionID,
					AuthenticationData: "{}",
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

		if uiLayoutInfo.GetEntry() == "chars" || uiLayoutInfo.GetEntry() == "chars_password" {
			go func() {
				// TODO: without password
				out, err := readPasswordWithContext(int(os.Stdin.Fd()), termCtx, uiLayoutInfo.GetEntry() == "chars_password")

				// No more processing if wait IsAuthorized has been answered.
				select {
				case <-termCtx.Done():
					return
				default:
				}

				// Immediately cancel wait goroutine, we won't care about its result
				cancelWait()

				if err != nil {
					results <- result{
						iaResp: nil,
						err:    err,
					}
				}

				authData := "{}"
				challenge := string(out)
				if challenge != "" {
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
		}

		r := <-results
		if r.err != nil {
			fmt.Println("ERROR: " + r.err.Error())
			return C.PAM_SYSTEM_ERR
		}
		iaResp = r.iaResp

	case "qrcode":
		l := uiLayoutInfo.GetLabel()
		if l != "" {
			fmt.Println(l)
		}
		qrCode, err := qrcode.New(uiLayoutInfo.GetContent(), qrcode.Medium)
		if err != nil {
			fmt.Println("Error generating QR code:", err)
			return C.PAM_SYSTEM_ERR
		}
		asciiQR := qrCode.ToSmallString(false)
		fmt.Println(asciiQR)

		iaReq := &authd.IARequest{
			SessionId:          sessionID,
			AuthenticationData: "{}",
		}
		iaResp, err = client.IsAuthorized(context.TODO(), iaReq)
		if err != nil {
			fmt.Println("ERROR QR CODE " + err.Error())
			return C.PAM_SYSTEM_ERR
		}
	}

	switch strings.ToLower(iaResp.Access) {
	case "denied":
		fmt.Println("Access Denied")
		return C.PAM_AUTH_ERR
	case "allowed":
		fmt.Println("Welcome")
		for k, v := range iaResp.UserInfo {
			fmt.Printf("Key: %s, Value: %s\n", k, v)
		}
	default:
		// Invalid response
		fmt.Printf("Error: Invalid Response: %v", iaResp.Access)
		return C.PAM_SYSTEM_ERR
	}

	// Set broker in env for not reasking in current session.
	os.Setenv("AUTHD_CURRENT_BROKER", brokerName)
	return C.PAM_SUCCESS
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

func selectBroker(brokersInfo []*authd.BrokersInfo_BrokerInfo) (name string) {
	// Print available brokers
	fmt.Println("1 - Local account")
	brokerChoices := make(map[int]string)
	for i, b := range brokersInfo {
		fmt.Printf("%d - %s, %s\n", i+2, b.Name, b.GetBrandIcon())
		brokerChoices[i+2] = b.Name
	}

	// Select broker
	var input string
	var chosenBroker int
	for {
		fmt.Printf("Select broker: ")
		_, err := fmt.Scanln(&input)
		if err != nil {
			fmt.Printf("ERROR: can't read stdin: %v", err)
			return ""
		}

		if input == "" || strings.ToLower(input) == "q" {
			input = "1"
			break
		}

		chosenBroker, err = strconv.Atoi(input)
		if err != nil || chosenBroker < 1 || chosenBroker > len(brokersInfo)+1 {
			fmt.Println("Error: invalid entry. Try again or press q.")
			continue
		}
		break
	}

	if chosenBroker == 1 {
		return ""
	}

	return brokerChoices[chosenBroker]
}

func selectAuthenticationMode(authModes []*authd.GetAuthenticationModesResponse_AuthenticationMode) (name string) {

	if len(authModes) < 1 {
		return ""
	}

	if len(authModes) == 1 {
		return authModes[0].Name
	}

	// Print available authentication modes
	authModeChoices := make(map[int]string)
	for i, m := range authModes {
		fmt.Printf("%d - %s\n", i+1, m.Label)
		authModeChoices[i+1] = m.Name
	}

	// Select auth modes
	var input string
	var chosenAuthMode int
	for {
		fmt.Printf("Select authentication mode: ")
		_, err := fmt.Scanln(&input)
		if err != nil {
			fmt.Printf("ERROR: can't read stdin %v", err)
			return ""
		}

		if input == "" || strings.ToLower(input) == "q" {
			input = "1"
			break
		}

		chosenAuthMode, err = strconv.Atoi(input)
		if err != nil || chosenAuthMode < 1 || chosenAuthMode > len(authModes) {
			fmt.Println("Error: invalid entry. Try again or press q.")
			continue
		}
		break
	}

	return authModeChoices[chosenAuthMode]
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
			default:
				ret = append(ret, buf[0])
			}
			continue
		}
	}
}

func main() {

}
