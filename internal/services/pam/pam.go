// Package pam implements the pam grpc service protocol to the daemon.
package pam

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/brokers/responses"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/newusers"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/decorate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

var _ authd.PAMServer = Service{}

// Service is the implementation of the PAM module service.
type Service struct {
	userManager   *newusers.Manager
	brokerManager *brokers.Manager

	authd.UnimplementedPAMServer
}

// NewService returns a new PAM GRPC service.
func NewService(ctx context.Context, userManager *newusers.Manager, brokerManager *brokers.Manager) Service {
	log.Debug(ctx, "Building new GRPC PAM service")

	return Service{
		userManager:   userManager,
		brokerManager: brokerManager,
	}
}

// AvailableBrokers returns the list of all brokers with their details.
func (s Service) AvailableBrokers(ctx context.Context, _ *authd.Empty) (*authd.ABResponse, error) {
	var r authd.ABResponse

	for _, b := range s.brokerManager.AvailableBrokers() {
		b := b
		r.BrokersInfos = append(r.BrokersInfos, &authd.ABResponse_BrokerInfo{
			Id:        b.ID,
			Name:      b.Name,
			BrandIcon: &b.BrandIconPath,
		})
	}

	return &r, nil
}

// GetPreviousBroker returns the previous broker set for a given user, if any.
func (s Service) GetPreviousBroker(ctx context.Context, req *authd.GPBRequest) (*authd.GPBResponse, error) {
	if b := s.brokerManager.BrokerForUser(req.GetUsername()); b != nil {
		return &authd.GPBResponse{PreviousBroker: &b.ID}, nil
	}

	brokerID, err := s.userManager.BrokerForUser(req.GetUsername())
	if err != nil {
		log.Infof(ctx, "Could not get previous broker for user %q from cache: %v", req.GetUsername(), err)
		return &authd.GPBResponse{}, nil
	}

	// Updates manager memory to stop needing to query the database for the broker.
	if err = s.brokerManager.SetDefaultBrokerForUser(brokerID, req.GetUsername()); err != nil {
		log.Warningf(ctx, "Last broker used by %q is not available: %v", req.GetUsername(), err)
		return &authd.GPBResponse{}, nil
	}

	return &authd.GPBResponse{PreviousBroker: &brokerID}, nil
}

// SelectBroker starts a new session and selects the requested broker for the user.
func (s Service) SelectBroker(ctx context.Context, req *authd.SBRequest) (resp *authd.SBResponse, err error) {
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

	// Create a session and Memorize selected broker for it.
	sessionID, encryptionKey, err := s.brokerManager.NewSession(brokerID, username, lang)
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
	if access == responses.AuthGranted {
		var user users.UserInfo
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			return nil, fmt.Errorf("user data from broker invalid: %v", err)
		}

		if err := s.userManager.UpdateUser(user); err != nil {
			return nil, err
		}

		if err := user.UpdateLocalGroups(); err != nil {
			return nil, err
		}

		// The data is not the message for the user then.
		data = ""
	}

	return &authd.IAResponse{
		Access: access,
		Msg:    data,
	}, nil
}

// SetDefaultBrokerForUser sets the default broker for the given user.
func (s Service) SetDefaultBrokerForUser(ctx context.Context, req *authd.SDBFURequest) (empty *authd.Empty, err error) {
	defer decorate.OnError(&err, "can't set default broker %q for user %q", req.GetBrokerId(), req.GetUsername())

	if req.GetUsername() == "" {
		return nil, status.Error(codes.InvalidArgument, "no user name given")
	}

	if err = s.brokerManager.SetDefaultBrokerForUser(req.GetBrokerId(), req.GetUsername()); err != nil {
		return &authd.Empty{}, err
	}

	if err = s.userManager.UpdateBrokerForUser(req.GetUsername(), req.GetBrokerId()); err != nil {
		return &authd.Empty{}, err
	}

	return &authd.Empty{}, nil
}

// EndSession asks the broker associated with the sessionID to end the session.
func (s Service) EndSession(ctx context.Context, req *authd.ESRequest) (empty *authd.Empty, err error) {
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

	return &authd.UILayout{
		Type:    typ,
		Label:   &label,
		Entry:   &entry,
		Button:  &button,
		Wait:    &wait,
		Content: &content,
	}
}
