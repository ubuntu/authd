// Package pam implements the pam grpc service protocol to the daemon.
package pam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/user"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/services/permissions"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/decorate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

var _ authd.PAMServer = Service{}

// Service is the implementation of the PAM module service.
type Service struct {
	userManager       *users.Manager
	brokerManager     *brokers.Manager
	permissionManager *permissions.Manager

	authd.UnimplementedPAMServer
}

// NewService returns a new PAM GRPC service.
func NewService(ctx context.Context, userManager *users.Manager, brokerManager *brokers.Manager, permissionManager *permissions.Manager) Service {
	log.Debug(ctx, "Building new GRPC PAM service")

	return Service{
		userManager:       userManager,
		brokerManager:     brokerManager,
		permissionManager: permissionManager,
	}
}

// AvailableBrokers returns the list of all brokers with their details.
func (s Service) AvailableBrokers(ctx context.Context, _ *authd.Empty) (*authd.ABResponse, error) {
	var r authd.ABResponse

	for _, b := range s.brokerManager.AvailableBrokers() {
		r.BrokersInfos = append(r.BrokersInfos, &authd.ABResponse_BrokerInfo{
			Id:        b.ID,
			Name:      b.Name,
			BrandIcon: &b.BrandIconPath,
		})
	}

	return &r, nil
}

// GetPreviousBroker returns the previous broker set for a given user, if any.
// If the user is not in our cache, it will try to check if it’s on the system, and return then "local".
func (s Service) GetPreviousBroker(ctx context.Context, req *authd.GPBRequest) (gpbr *authd.GPBResponse, err error) {
	defer redactError(&err)

	// Use in memory cache first
	if b := s.brokerManager.BrokerForUser(req.GetUsername()); b != nil {
		return &authd.GPBResponse{PreviousBroker: b.ID}, nil
	}

	// Load from database cache.
	brokerID, err := s.userManager.BrokerForUser(req.GetUsername())
	// User is not in our cache.
	if err != nil && errors.Is(err, users.ErrNoDataFound{}) {
		// User not acccessible through NSS, first time login or no valid user. Anyway, no broker selected.
		if _, err := user.Lookup(req.GetUsername()); err != nil {
			log.Debugf(ctx, "User %q is unknown", req.GetUsername())
			return &authd.GPBResponse{}, nil
		}

		// We could resolve the user through NSS, which means then that another non authd service
		// service (passwd, winbind, sss…) is handling that user.
		brokerID = brokers.LocalBrokerName
	} else if err != nil {
		log.Infof(ctx, "Could not get previous broker for user %q from cache: %v", req.GetUsername(), err)
		return &authd.GPBResponse{}, nil
	}

	// No error but the brokerID is empty (broker in cache but default broker not stored yet due no successful login)
	if brokerID == "" {
		log.Infof(ctx, "No assigned broker for user %q from cache", req.GetUsername())
		return &authd.GPBResponse{}, nil
	}

	// Updates manager memory to stop needing to query the database for the broker.
	if err = s.brokerManager.SetDefaultBrokerForUser(brokerID, req.GetUsername()); err != nil {
		log.Warningf(ctx, "Last broker used by %q is not available, letting the user selecting one: %v", req.GetUsername(), err)
		return &authd.GPBResponse{}, nil
	}

	return &authd.GPBResponse{
		PreviousBroker: brokerID,
	}, nil
}

// SelectBroker starts a new session and selects the requested broker for the user.
func (s Service) SelectBroker(ctx context.Context, req *authd.SBRequest) (resp *authd.SBResponse, err error) {
	defer redactError(&err)
	defer decorate.OnError(&err, "can't start authentication transaction")

	username := req.GetUsername()
	brokerID := req.GetBrokerId()
	lang := req.GetLang()

	if username == "" {
		return nil, status.Error(codes.InvalidArgument, "no user name provided")
	}
	if brokerID == "" {
		return nil, status.Error(codes.InvalidArgument, "no broker selected")
	}
	if lang == "" {
		lang = "C"
	}

	var mode string
	switch req.GetMode() {
	case authd.SessionMode_AUTH:
		mode = "auth"
	case authd.SessionMode_PASSWD:
		mode = "passwd"
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid session mode")
	}

	// Create a session and Memorize selected broker for it.
	sessionID, encryptionKey, err := s.brokerManager.NewSession(brokerID, username, lang, mode)
	if err != nil {
		return nil, err
	}

	return &authd.SBResponse{
		SessionId:     sessionID,
		EncryptionKey: encryptionKey,
	}, err
}

// GetAuthenticationModes fetches a list of authentication modes supported by the broker depending on the session information.
func (s Service) GetAuthenticationModes(ctx context.Context, req *authd.GAMRequest) (resp *authd.GAMResponse, err error) {
	defer redactError(&err)
	defer decorate.OnError(&err, "could not get authentication modes")

	sessionID := req.GetSessionId()
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "no session ID provided")
	}

	broker, err := s.brokerManager.BrokerFromSessionID(sessionID)
	if err != nil {
		return nil, err
	}

	var layouts []map[string]string
	for _, l := range req.GetSupportedUiLayouts() {
		layout, err := uiLayoutToMap(l)
		if err != nil {
			return nil, err
		}
		layouts = append(layouts, layout)
	}

	authenticationModes, err := broker.GetAuthenticationModes(ctx, sessionID, layouts)
	if err != nil {
		return nil, err
	}

	var authModes []*authd.GAMResponse_AuthenticationMode
	for _, a := range authenticationModes {
		authModes = append(authModes, &authd.GAMResponse_AuthenticationMode{
			Id:    a["id"],
			Label: a["label"],
		})
	}

	return &authd.GAMResponse{
		AuthenticationModes: authModes,
	}, nil
}

// SelectAuthenticationMode set given authentication mode as selected for this sessionID to the broker.
func (s Service) SelectAuthenticationMode(ctx context.Context, req *authd.SAMRequest) (resp *authd.SAMResponse, err error) {
	defer redactError(&err)
	defer decorate.OnError(&err, "can't select authentication mode")

	sessionID := req.GetSessionId()
	authenticationModeID := req.GetAuthenticationModeId()

	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "no session ID provided")
	}
	if authenticationModeID == "" {
		return nil, status.Error(codes.InvalidArgument, "no authentication mode provided")
	}

	broker, err := s.brokerManager.BrokerFromSessionID(sessionID)
	if err != nil {
		return nil, err
	}

	uiLayoutInfo, err := broker.SelectAuthenticationMode(ctx, sessionID, authenticationModeID)
	if err != nil {
		return nil, err
	}

	return &authd.SAMResponse{
		UiLayoutInfo: mapToUILayout(uiLayoutInfo),
	}, nil
}

// IsAuthenticated returns broker answer to authentication request.
func (s Service) IsAuthenticated(ctx context.Context, req *authd.IARequest) (resp *authd.IAResponse, err error) {
	defer redactError(&err)
	defer decorate.OnError(&err, "can't check authentication")

	sessionID := req.GetSessionId()
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "no session ID provided")
	}

	broker, err := s.brokerManager.BrokerFromSessionID(sessionID)
	if err != nil {
		return nil, err
	}

	authenticationDataJSON, err := protojson.Marshal(req.GetAuthenticationData())
	if err != nil {
		return nil, err
	}

	access, data, err := broker.IsAuthenticated(ctx, sessionID, string(authenticationDataJSON))
	if err != nil {
		return nil, err
	}

	// Update database and local groups on granted auth.
	if access == brokers.AuthGranted {
		var user users.UserInfo
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			return nil, fmt.Errorf("user data from broker invalid: %v", err)
		}

		if err := s.userManager.UpdateUser(user); err != nil {
			return nil, err
		}

		// The data is not the message for the user then.
		data = ""
	}

	data = redactMessage(data)
	return &authd.IAResponse{
		Access: access,
		Msg:    data,
	}, nil
}

// SetDefaultBrokerForUser sets the default broker for the given user.
func (s Service) SetDefaultBrokerForUser(ctx context.Context, req *authd.SDBFURequest) (empty *authd.Empty, err error) {
	defer redactError(&err)
	defer decorate.OnError(&err, "can't set default broker %q for user %q", req.GetBrokerId(), req.GetUsername())

	if req.GetUsername() == "" {
		return nil, status.Error(codes.InvalidArgument, "no user name given")
	}

	if err = s.brokerManager.SetDefaultBrokerForUser(req.GetBrokerId(), req.GetUsername()); err != nil {
		return &authd.Empty{}, err
	}

	if req.GetBrokerId() == brokers.LocalBrokerName {
		return &authd.Empty{}, nil
	}

	if err = s.userManager.UpdateBrokerForUser(req.GetUsername(), req.GetBrokerId()); err != nil {
		return &authd.Empty{}, err
	}

	return &authd.Empty{}, nil
}

// EndSession asks the broker associated with the sessionID to end the session.
func (s Service) EndSession(ctx context.Context, req *authd.ESRequest) (empty *authd.Empty, err error) {
	defer redactError(&err)
	defer decorate.OnError(&err, "could not abort session")

	sessionID := req.GetSessionId()
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "no session id given")
	}

	return &authd.Empty{}, s.brokerManager.EndSession(sessionID)
}

func uiLayoutToMap(layout *authd.UILayout) (mapLayout map[string]string, err error) {
	if layout.GetType() == "" {
		return nil, fmt.Errorf("invalid layout option: type is required, got: %v", layout)
	}
	r := map[string]string{"type": layout.GetType()}
	if l := layout.GetLabel(); l != "" {
		r["label"] = l
	}
	if b := layout.GetButton(); b != "" {
		r["button"] = b
	}
	if w := layout.GetWait(); w != "" {
		r["wait"] = w
	}
	if e := layout.GetEntry(); e != "" {
		r["entry"] = e
	}
	if c := layout.GetContent(); c != "" {
		r["content"] = c
	}
	if c := layout.GetCode(); c != "" {
		r["code"] = c
	}
	return r, nil
}

// mapToUILayout generates an UILayout from the input map.
func mapToUILayout(layout map[string]string) (r *authd.UILayout) {
	typ := layout["type"]
	label := layout["label"]
	entry := layout["entry"]
	button := layout["button"]
	wait := layout["wait"]
	content := layout["content"]
	code := layout["code"]

	return &authd.UILayout{
		Type:    typ,
		Label:   &label,
		Entry:   &entry,
		Button:  &button,
		Wait:    &wait,
		Content: &content,
		Code:    &code,
	}
}

// errGeneric is the error to return when the broker returns an error.
//
// This error is returned to the client to prevent information leaks.
var errGeneric = errors.New("authentication failure")

// redactError replaces the error with a generic one to prevent information leaks.
//
// Since the error messages contain useful information for debugging, the original error message
// is written in the system logs.
func redactError(err *error) {
	if *err == nil {
		return
	}
	slog.Debug(fmt.Sprintf("%v", err))
	*err = errGeneric
}

// genericErrorMessage is the message to return when the broker returns an error message.
//
// This message is returned to the client to prevent information leaks.
const genericErrorMessage string = `{"message":"authentication failure"}`

// redactMessage replaces the message with a generic one to prevent information leaks.
//
// Since the message contains useful information for debugging, the original message is written
// in the system logs.
func redactMessage(msg string) string {
	if msg == "{}" || msg == "" {
		return msg
	}
	slog.Debug(fmt.Sprintf("Got broker message: %q", msg))
	return genericErrorMessage
}
