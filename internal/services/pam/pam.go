// Package pam implements the pam grpc service protocol to the daemon.
package pam

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/log"
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
		case "text":
			if layout.GetPassword() == "optional" {
				authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
					Name:  "password",
					Label: "Password authentication",
				})
				if layout.GetDigits() == "optional" {
					if layout.GetButton() == "" {
						authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
							Name:  "pincode",
							Label: "Pin code",
						})
					}
				}
			}
			if layout.GetButton() == "optional" {
				authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
					Name:  "topt",
					Label: "Authentication code",
				})
			}

		case "message":
			authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
				Name:  "phoneack1",
				Label: "Use your phone +33…",
			})
			authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
				Name:  "phoneack2",
				Label: "Use your phone +33",
			})
			authModes = append(authModes, &authd.GetAuthenticationModesResponse_AuthenticationMode{
				Name:  "fidodevice1",
				Label: "Use your fido device",
			})
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

	// Store selected mode
	currentSessionInfo := currentSessions[req.SessionId]
	currentSessionInfo.selectedMode = authMode
	currentSessions[req.SessionId] = currentSessionInfo

	trueStr := "true"

	// populate UI options based on selected authentication mode
	var layoutInfo authd.UILayout
	switch authMode {
	case "password":
		promptMsg := "Gimme your password:"
		layoutInfo = authd.UILayout{
			Type:     "text",
			Label:    &promptMsg,
			Password: &trueStr,
		}
	case "pincode":
		promptMsg := "Enter your pin code:"
		layoutInfo = authd.UILayout{
			Type:     "text",
			Label:    &promptMsg,
			Password: &trueStr,
			Digits:   &trueStr,
		}
	case "topt":
		promptMsg := "Enter your one time credential:"
		buttonMsg := "Resend sms"
		layoutInfo = authd.UILayout{
			Type:   "text",
			Label:  &promptMsg,
			Button: &buttonMsg,
		}
	case "phoneack1":
		promptMsg := "Unlock your phone +33… or accept request on web interface:"
		layoutInfo = authd.UILayout{
			Type:  "message",
			Label: &promptMsg,
		}
		// TODO: send notification to phone1
	case "phoneack2":
		promptMsg := "Unlock your phone +1… or accept request on web interface:"
		layoutInfo = authd.UILayout{
			Type:  "message",
			Label: &promptMsg,
		}
		// TODO: send notification to phone2
	case "fidodevice1":
		promptMsg := "Plug your fido device and press with your thumb"
		layoutInfo = authd.UILayout{
			Type:  "message",
			Label: &promptMsg,
		}
	case "qrcodewithtypo":
		promptMsg := "Enter the following code after flashing the address: 1337"
		url := "https://ubuntu.com"
		layoutInfo = authd.UILayout{
			Type:  "qrcode",
			Label: &promptMsg,
			Text:  &url,
		}
	}

	return &authd.SAMResponse{
		UiLayoutInfo: &layoutInfo,
	}, nil
}

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

	case "topt":
		if data["challenge"] != "temporary pass" {
			return authDeniedResp, nil
		}

	case "phoneack1":
		// TODO: send notification to phone1 and wait on server signal to return if OK or not
		time.Sleep(5 * time.Second)

	case "phoneack2":
		// This one is failing remotely as an example
		time.Sleep(2 * time.Second)
		return authDeniedResp, nil

	case "fidodevice1":
		// TODO: direct exchange with the FIDO device
		time.Sleep(5 * time.Second)

	case "qrcodewithtypo":
		// TODO: connexion with remote server to check that the correct code was entered
		time.Sleep(10 * time.Second)
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
