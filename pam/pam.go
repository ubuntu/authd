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

	"github.com/skip2/go-qrcode"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/log"
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
	gamReq := &authd.GetAuthenticationModesRequest{
		Broker:   brokerName,
		Username: user,
		Lang:     lang,
		SupportedUiLayouts: []*authd.UILayout{
			{
				Type:     "text",
				Label:    &required,
				Password: &optional,
				Digits:   &optional,
			},
			{
				Type:  "message",
				Label: &required,
			},
			{
				Type:  "qrcode",
				Label: &optional,
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

	// Show UI with additional info and select different mode of action
	var challenge string
	switch uiInfo.GetUiLayoutInfo().Type {
	case "text":
		prompt := uiInfo.GetUiLayoutInfo().GetLabel()
		if prompt == "" {
			// TODO error
			return C.PAM_SYSTEM_ERR
		}
		if !strings.HasSuffix(prompt, " ") {
			prompt = fmt.Sprintf("%s ", prompt)
		}

		// TODO: clear or not clear text depending on password
		if strings.ToLower(uiInfo.GetUiLayoutInfo().GetPassword()) == "true" {
			challenge, err = getPassword(prompt)
		} else {
			fmt.Printf(prompt)
			_, err = fmt.Scanln(&challenge)
		}
		if err != nil {
			fmt.Printf("Error while getting password: %v", err)
			return C.PAM_AUTH_ERR
		}
	case "message":
		l := uiInfo.GetUiLayoutInfo().Label
		if l == nil {
			// TODO error
			return C.PAM_SYSTEM_ERR
		}
		fmt.Printf(*l)
	case "qrcode":
		l := uiInfo.GetUiLayoutInfo().GetLabel()
		if l != "" {
			fmt.Println(l)
		}
		qrCode, err := qrcode.New(uiInfo.GetUiLayoutInfo().GetText(), qrcode.Medium)
		if err != nil {
			fmt.Println("Error generating QR code:", err)
			return C.PAM_SYSTEM_ERR
		}
		asciiQR := qrCode.ToSmallString(false)
		fmt.Println(asciiQR)
	}

	authData := "{}"
	if challenge != "" {
		authData = fmt.Sprintf(`{"challenge": "%s"}`, challenge)
	}

	// Validate challenge with Broker
	iaReq := &authd.IARequest{
		SessionId:          sessionID,
		AuthenticationData: authData,
	}
	iaResp, err := client.IsAuthorized(context.TODO(), iaReq)
	if err != nil {
		fmt.Printf("Error: cannot get authorization: %v", err)
		return C.PAM_AUTH_ERR
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

func main() {

}
