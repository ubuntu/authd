package brokers

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
)

type sessionInfo struct {
	username     string
	selectedMode string
	lang         string
	allModes     map[string]map[string]string
}

type exampleBroker struct {
	currentSessions      map[string]sessionInfo
	userLastSelectedMode map[string]string
}

var (
	users = map[string]string{
		"user1": `
		{
			"uid": "4245874",
			"name": "My user",
			"groups": {
				"group1": {
					"name": "Group 1",
					"gid": "3884"
				},
				"group2": {
					"name": "Group 2",
					"gid": "4884"
				}
			}
		}
	`,
		"user2": `
		{
			"uid": "33333",
			"name": "My secondary user",
			"groups": {
				"group2": {
					"name": "Group 2",
					"gid": "4884"
				}
			}
		}
	`,
	}
)

const (
	brokerEncryptionKey = "encryptionkey"
)

func newExampleBroker(name string) (b *exampleBroker, fullName, brandIcon string, err error) {
	return &exampleBroker{
		currentSessions:        make(map[string]sessionInfo),
		currentSessionsMu:      sync.Mutex{},
		userLastSelectedMode:   make(map[string]string),
		userLastSelectedModeMu: sync.Mutex{},
	}, strings.ReplaceAll(name, "_", " "), fmt.Sprintf("/usr/share/brokers/%s.png", name), nil
}

func (b *exampleBroker) GetAuthenticationModes(ctx context.Context, username, lang string, supportedUiLayouts []map[string]string) (sessionID, encryptionKey string, authenticationModes []map[string]string, err error) {
	sessionID = uuid.New().String()

	//var candidatesAuthenticationModes []map[string]string
	allModes := make(map[string]map[string]string)
	for _, layout := range supportedUiLayouts {
		switch layout["type"] {
		case "form":
			if layout["entry"] != "" {
				supportedEntries := strings.Split(strings.TrimPrefix(layout["entry"], "optional:"), ",")
				if slices.Contains(supportedEntries, "chars_password") {
					allModes["password"] = map[string]string{
						"selection_label": "Password authentication",
						"ui": mapToJson(map[string]string{
							"type":  "form",
							"label": "Gimme your password",
							"entry": "chars_password",
						}),
					}
				}
				if slices.Contains(supportedEntries, "digits") {
					allModes["pincode"] = map[string]string{
						"selection_label": "Pin code",
						"ui": mapToJson(map[string]string{
							"type":  "form",
							"label": "Enter your pin code",
							"entry": "digits",
						}),
					}
				}
				if slices.Contains(supportedEntries, "chars") && layout["wait"] != "" {
					allModes[fmt.Sprintf("entry_or_wait_for_%s_gmail.com", username)] = map[string]string{
						"selection_label": fmt.Sprintf("Send URL to %s@gmail.com", username),
						"email":           fmt.Sprintf("%s@gmail.com", username),
						"ui": mapToJson(map[string]string{
							"type":  "form",
							"label": fmt.Sprintf("Click on the link received at %s@gmail.com or enter the code:", username),
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
						"ui": mapToJson(map[string]string{
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
						"ui": mapToJson(map[string]string{
							"type":  "form",
							"label": "Enter your one time credential",
							"entry": "chars",
						}),
					}
				}

				allModes["phoneack1"] = map[string]string{
					"selection_label": "Use your phone +33…",
					"phone":           "+33…",
					"ui": mapToJson(map[string]string{
						"type":  "form",
						"label": "Unlock your phone +33… or accept request on web interface:",
						"wait":  "true",
					}),
				}

				allModes["phoneack2"] = map[string]string{
					"selection_label": "Use your phone +1…",
					"phone":           "+1…",
					"ui": mapToJson(map[string]string{
						"type":  "form",
						"label": "Unlock your phone +1… or accept request on web interface",
						"wait":  "true",
					}),
				}

				allModes["fidodevice1"] = map[string]string{
					"selection_label": "Use your fido device foo",
					"ui": mapToJson(map[string]string{
						"type":  "form",
						"label": "Plug your fido device and press with your thumb",
						"wait":  "true",
					}),
				}
			}

		case "qrcode":
			allModes["qrcodewithtypo"] = map[string]string{
				"selection_label": "Use a QR code",
				"ui": mapToJson(map[string]string{
					"type":  "qrcode",
					"label": "Enter the following code after flashing the address: ",
					"wait":  "true",
				}),
			}

		case "webview":
			// This broker does not support webview
		}
	}

	// Sort in preference order. We want by default password as first and potentially last selection too.
	lastSelection := b.userLastSelectedMode[username]
	if _, exists := allModes[lastSelection]; !exists {
		lastSelection = ""
	}

	var allNames []string
	for n := range allModes {
		if n == "password" || n == lastSelection {
			continue
		}
		allNames = append(allNames, n)
	}
	sort.Strings(allNames)
	if lastSelection != "" && lastSelection != "password" {
		allNames = append([]string{lastSelection, "password"}, allNames...)
	} else {
		allNames = append([]string{"password"}, allNames...)
	}

	for _, name := range allNames {
		authMode := allModes[name]
		authenticationModes = append(authenticationModes, map[string]string{
			"name":  name,
			"label": authMode["selection_label"],
		})
	}

	b.currentSessions[sessionID] = sessionInfo{
		username: username,
		lang:     lang,
		allModes: allModes,
	}

	return sessionID, brokerEncryptionKey, authenticationModes, nil
}

func (b *exampleBroker) SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error) {
	// Ensure session ID is an active one.

	sessionInfo, inprogress := b.currentSessions[sessionID]
	if !inprogress {
		return nil, fmt.Errorf("%s is not a current transaction", sessionID)
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
	case "phoneack1", "phoneack2":
		// send request to sessionInfo.allModes[authenticationModeName]["phone"]
	case "fidodevice1":
		// start transaction with fideo device
	case "qrcodewithtypo":
		// generate the url and finish the prompt on the fly.
		uiLayoutInfo["content"] = "https://ubuntu.com"
		uiLayoutInfo["label"] = uiLayoutInfo["label"] + "1337"
	}

	// Store selected mode
	sessionInfo.selectedMode = authenticationModeName
	b.currentSessions[sessionID] = sessionInfo

	return uiLayoutInfo, nil
}

func (b *exampleBroker) IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access, infoUser string, err error) {
	sessionInfo, inprogress := b.currentSessions[sessionID]
	if !inprogress {
		return "", "", fmt.Errorf("%s is not a current transaction", sessionID)
	}

	//authenticationData = decryptAES([]byte(brokerEncryptionKey), authenticationData)
	var authData map[string]string
	if authenticationData != "" {
		if err := json.Unmarshal([]byte(authenticationData), &authData); err != nil {
			return "", "", errors.New("authentication data is not a valid json value")
		}
	}

	authDeniedResp := "denied"

	// Note that the "wait" authentication can be cancelled and switch to another mode with a challenge.
	// Take into account the cancellation.
	switch sessionInfo.selectedMode {
	case "password":
		if authData["challenge"] != "goodpass" {
			return authDeniedResp, "", nil
		}

	case "pincode":
		if authData["challenge"] != "4242" {
			return authDeniedResp, "", nil
		}

	case "totp_with_button", "totp":
		if authData["challenge"] != "temporary pass" {
			return authDeniedResp, "", nil
		}

	case "phoneack1":
		if authData["wait"] != "true" {
			return authDeniedResp, "", nil
		}
		// Send notification to phone1 and wait on server signal to return if OK or not
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return authDeniedResp, "", nil
		}

	case "phoneack2":
		if authData["wait"] != "true" {
			return authDeniedResp, "", nil
		}

		// This one is failing remotely as an example
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return authDeniedResp, "", nil
		}
		return authDeniedResp, "", nil

	case "fidodevice1":
		if authData["wait"] != "true" {
			return authDeniedResp, "", nil
		}

		// simulate direct exchange with the FIDO device
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return authDeniedResp, "", nil
		}

	case "qrcodewithtypo":
		if authData["wait"] != "true" {
			return authDeniedResp, "", nil
		}
		// Simulate connexion with remote server to check that the correct code was entered
		select {
		case <-time.After(4 * time.Second):
		case <-ctx.Done():
			return authDeniedResp, "", nil
		}
	}

	// this case name was dynamically generated
	if strings.HasPrefix(sessionInfo.selectedMode, "entry_or_wait_for_") {
		// do we have a challenge sent or should we just wait?
		if authData["challenge"] != "" {
			// validate challenge given manually by the user
			if authData["challenge"] != "aaaaa" {
				return authDeniedResp, "", nil
			}
		} else if authData["wait"] == "true" {
			// we are simulating clicking on the url signal received by the broker
			// this can be cancelled to resend a challenge
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return authDeniedResp, "", nil
			}
		} else {
			return authDeniedResp, "", nil
		}
	}

	userInfo, exists := users[sessionInfo.username]
	if !exists {
		return authDeniedResp, "", nil
	}

	// Store last successful authentication mode for this user in the broker.
	b.userLastSelectedMode[sessionInfo.username] = sessionInfo.selectedMode

	return "allowed", userInfo, nil
}

func mapToJson(input map[string]string) string {
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
// This has to be changed in the final implementation
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
// This has to be changed in the final implementation
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
