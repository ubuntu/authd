// Package pam implements the pam grpc service protocol to the daemon.
package pam

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/log"
	"golang.org/x/exp/slices"
)

var _ authd.PAMServer = Service{}

// Service is the implementation of the PAM module service.
type Service struct {
	brokerManager *brokers.Manager

	authd.UnimplementedPAMServer
}

// NewService returns a new PAM GRPC service.
func NewService(ctx context.Context, brokerManager *brokers.Manager) Service {
	log.Debug(ctx, "Building new GRPC PAM service")

	return Service{
		brokerManager: brokerManager,
	}
}

// GetBrokersInfo returns the list of all brokers with their details.
func (s Service) GetBrokersInfo(context.Context, *authd.Empty) (*authd.BrokersInfo, error) {
	var r authd.BrokersInfo

	brandIcon1, brandIcon2 := "/my/image/broker1", "/my/image/broker2"
	r.BrokersInfos = append(r.BrokersInfos, &authd.BrokersInfo_BrokerInfo{
		Name:      "warthogs",
		BrandIcon: &brandIcon1,
	}, &authd.BrokersInfo_BrokerInfo{
		Name:      "no named",
		BrandIcon: &brandIcon2,
	})

	/*for _, b := range s.brokerManager.AvailableBrokers() {
		name, brandIcon := b.Info()
		r.BrokersInfos = append(r.BrokersInfos, &authd.BrokersInfo_BrokerInfo{
			Name:      name,
			BrandIcon: brandIcon,
		})
	}*/

	return &r, nil
}

type sessionInfo struct {
	username     string
	selectedMode string
}

var currentSessions = make(map[string]sessionInfo)

func (s Service) GetAuthenticationModes(ctx context.Context, req *authd.GetAuthenticationModesRequest) (resp *authd.GetAuthenticationModesResponse, err error) {
	_ = req.GetBroker()
	_ = req.GetLang()

	var authModes []*authd.GetAuthenticationModesResponse_AuthenticationMode

	for _, layout := range req.GetSupportedUiLayouts() {
		switch layout.Type {
		case "form":
			if layout.GetEntry() != "" {
				supportedEntries := strings.Split(strings.TrimPrefix(layout.GetEntry(), "optional:"), ",")
				if slices.Contains(supportedEntries, "chars_password") {
					authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
						Name:  "password",
						Label: "Password authentication",
					})
				}
				if slices.Contains(supportedEntries, "digits") {
					authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
						Name:  "pincode",
						Label: "Pin code",
					})
				}
				if slices.Contains(supportedEntries, "chars") && layout.GetWait() != "" {
					authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
						Name:  "entry_or_wait_for_foogmail.com",
						Label: "Send URL to foo@gmail.com",
					})
				}
			}

			// The broker could parse the values, that are either true/false
			if layout.GetWait() != "" {
				if layout.GetButton() == "optional" {
					authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
						Name:  "topt_with_button",
						Label: "Authentication code",
					})
				} else {
					authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
						Name:  "topt",
						Label: "Authentication code",
					})
					authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
						Name:  "phoneack1",
						Label: "Use your phone +33…",
					})
					authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
						Name:  "phoneack2",
						Label: "Use your phone +1…",
					})
				}
				authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
					Name:  "fidodevice1",
					Label: "Use your fido device",
				})
			}

		case "qrcode":
			authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
				Name:  "qrcodewithtypo",
				Label: "Use a QR code",
			})
		case "webview":
			// This broker does not support webview
		}
	}

	// TODO authModes: sort in preference order

	generatedSessionID := "my-random-id"
	currentSessions[generatedSessionID] = sessionInfo{username: req.GetUsername()}

	return &authd.GetAuthenticationModesResponse{
		SessionId:           "my-random-id",
		EncryptionKey:       "my secret public encryption key",
		AuthenticationModes: authModes,
	}, nil
}

func (s Service) SelectAuthenticationMode(ctx context.Context, req *authd.SAMRequest) (resp *authd.SAMResponse, err error) {
	// Ensure session ID is an active one.

	authMode := req.GetAuthenticationModeName()
	fmt.Println("SELECT:", authMode, req.SessionId)

	// Store selected mode
	currentSessionInfo := currentSessions[req.SessionId]
	currentSessionInfo.selectedMode = authMode
	currentSessions[req.SessionId] = currentSessionInfo

	trueStr := "true"
	chars := "chars"
	charsPassword := "chars_password"
	digits := "digits"

	// populate UI options based on selected authentication mode
	var layoutInfo authd.UILayout
	switch authMode {
	case "password":
		promptMsg := "Gimme your password:"
		layoutInfo = authd.UILayout{
			Type:  "form",
			Label: &promptMsg,
			Entry: &charsPassword,
		}
	case "pincode":
		promptMsg := "Enter your pin code:"
		layoutInfo = authd.UILayout{
			Type:  "form",
			Label: &promptMsg,
			Entry: &digits,
		}
	case "entry_or_wait_for_foogmail.com":
		promptMsg := "Click on the link received at foo@gmail.com or enter the code:"
		layoutInfo = authd.UILayout{
			Type:  "form",
			Label: &promptMsg,
			Entry: &chars,
			Wait:  &trueStr,
		}
	case "topt_with_button":
		promptMsg := "Enter your one time credential:"
		buttonMsg := "Resend sms"
		layoutInfo = authd.UILayout{
			Type:   "form",
			Label:  &promptMsg,
			Button: &buttonMsg,
			Wait:   &trueStr,
		}
	case "topt":
		promptMsg := "Enter your one time credential:"
		layoutInfo = authd.UILayout{
			Type:  "form",
			Label: &promptMsg,
			Wait:  &trueStr,
		}
	case "phoneack1":
		promptMsg := "Unlock your phone +33… or accept request on web interface:"
		layoutInfo = authd.UILayout{
			Type:  "form",
			Label: &promptMsg,
			Wait:  &trueStr,
		}
	case "phoneack2":
		promptMsg := "Unlock your phone +1… or accept request on web interface:"
		layoutInfo = authd.UILayout{
			Type:  "form",
			Label: &promptMsg,
			Wait:  &trueStr,
		}
	case "fidodevice1":
		promptMsg := "Plug your fido device and press with your thumb"
		layoutInfo = authd.UILayout{
			Type:  "form",
			Label: &promptMsg,
			Wait:  &trueStr,
		}
	case "qrcodewithtypo":
		promptMsg := "Enter the following code after flashing the address: 1337"
		url := "https://ubuntu.com"
		layoutInfo = authd.UILayout{
			Type:    "qrcode",
			Label:   &promptMsg,
			Content: &url,
			Wait:    &trueStr,
		}
	}

	return &authd.SAMResponse{
		UiLayoutInfo: &layoutInfo,
	}, nil
}

// IsAuthorized returns broker answer to authorization request.
func (s Service) IsAuthorized(ctx context.Context, req *authd.IARequest) (resp *authd.IAResponse, err error) {
	sessionInfo := currentSessions[req.SessionId]
	authDeniedResp := &authd.IAResponse{Access: "denied"}

	users := make(map[string]string)
	users["user1"] = `
		"uid": "4245874",
		"name": "My user",
		"groups": [
			"group1": {
				"name": "Group 1",
				"gid": "3884"
			},
			"group2": {
				"name": "Group 2",
				"gid": "4884"
			},
		]
	`
	users["user2"] = `
		"uid": "3333333",
		"name": "My secondary user",
		"groups": [
			"group2": {
				"name": "Group 2",
				"gid": "4884"
			},
		]
	`

	data, err := authenticationData(req.GetAuthenticationData())
	if err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return authDeniedResp, err
	}

	switch currentSessions[req.SessionId].selectedMode {
	case "password":
		if data["challenge"] != "goodpass" {
			return authDeniedResp, nil
		}

	case "pincode":
		if data["challenge"] != "4242" {
			return authDeniedResp, nil
		}

	case "entry_or_wait_for_foogmail.com":
		// do we have a challenge sent or should we just wait?
		if data["challenge"] != "" {
			// validate challenge given manually by the user
			if data["challenge"] != "abcde" {
				return authDeniedResp, nil
			}
		} else {
			// we are simulating clicking on the url signal received by the broker
			time.Sleep(10 * time.Second)
		}

	case "topt_with_button", "topt":
		if data["challenge"] != "temporary pass" {
			return authDeniedResp, nil
		}

	case "phoneack1":
		// Send notification to phone1 and wait on server signal to return if OK or not
		time.Sleep(5 * time.Second)

	case "phoneack2":
		// This one is failing remotely as an example
		time.Sleep(2 * time.Second)
		return authDeniedResp, nil

	case "fidodevice1":
		// simulate direct exchange with the FIDO device
		time.Sleep(5 * time.Second)

	case "qrcodewithtypo":
		// Simulate connexion with remote server to check that the correct code was entered
		time.Sleep(4 * time.Second)
	}

	userInfo, exists := users[sessionInfo.username]
	if !exists {
		return authDeniedResp, nil
	}
	// TODO in authd itself: store userinfo here for nss
	_ = userInfo

	return &authd.IAResponse{Access: "allowed"}, nil
}

func authenticationData(jsonData string) (map[string]string, error) {
	var data map[string]string

	err := json.Unmarshal([]byte(jsonData), &data)
	if err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return nil, err
	}

	return data, nil
}
