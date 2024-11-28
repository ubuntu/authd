// Package pam implements the pam proto service protocol to the daemon.
package pam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/user"

	"github.com/ubuntu/authd/brokers/auth"
	"github.com/ubuntu/authd/brokers/layouts"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/proto"
	"github.com/ubuntu/authd/internal/services/permissions"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/decorate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

var _ proto.PAMServer = Service{}

// Service is the implementation of the PAM module service.
type Service struct {
	userManager       *users.Manager
	brokerManager     *brokers.Manager
	permissionManager *permissions.Manager

	proto.UnimplementedPAMServer
}

// NewService returns a new PAM proto service.
func NewService(ctx context.Context, userManager *users.Manager, brokerManager *brokers.Manager, permissionManager *permissions.Manager) Service {
	log.Debug(ctx, "Building new proto PAM service")

	return Service{
		userManager:       userManager,
		brokerManager:     brokerManager,
		permissionManager: permissionManager,
	}
}

// AvailableBrokers returns the list of all brokers with their details.
func (s Service) AvailableBrokers(ctx context.Context, _ *proto.Empty) (*proto.ABResponse, error) {
	var r proto.ABResponse

	for _, b := range s.brokerManager.AvailableBrokers() {
		r.BrokersInfos = append(r.BrokersInfos, &proto.ABResponse_BrokerInfo{
			Id:        b.ID,
			Name:      b.Name,
			BrandIcon: &b.BrandIconPath,
		})
	}

	return &r, nil
}

// GetPreviousBroker returns the previous broker set for a given user, if any.
// If the user is not in our cache, it will try to check if it’s on the system, and return then "local".
func (s Service) GetPreviousBroker(ctx context.Context, req *proto.GPBRequest) (*proto.GPBResponse, error) {
	// Use in memory cache first
	if b := s.brokerManager.BrokerForUser(req.GetUsername()); b != nil {
		return &proto.GPBResponse{PreviousBroker: b.ID}, nil
	}

	// Load from database cache.
	brokerID, err := s.userManager.BrokerForUser(req.GetUsername())
	// User is not in our cache.
	if err != nil && errors.Is(err, users.NoDataFoundError{}) {
		// FIXME: this part will not be here in the v2 API version, as we won’t have GetPreviousBroker and handle
		// autoselection silently in authd.
		// User not in cache, if there is only the local broker available, return this one without saving it.
		if len(s.brokerManager.AvailableBrokers()) == 1 {
			log.Debugf(ctx, "User %q is not handled by authd and only local broker: select it.", req.GetUsername())
			return &proto.GPBResponse{PreviousBroker: brokers.LocalBrokerName}, nil
		}

		// User not acccessible through NSS, first time login or no valid user. Anyway, no broker selected.
		if _, err := user.Lookup(req.GetUsername()); err != nil {
			log.Debugf(ctx, "User %q is unknown", req.GetUsername())
			return &proto.GPBResponse{}, nil
		}

		// We could resolve the user through NSS, which means then that another non authd service
		// service (passwd, winbind, sss…) is handling that user.
		brokerID = brokers.LocalBrokerName
	} else if err != nil {
		log.Infof(ctx, "Could not get previous broker for user %q from cache: %v", req.GetUsername(), err)
		return &proto.GPBResponse{}, nil
	}

	// No error but the brokerID is empty (broker in cache but default broker not stored yet due no successful login)
	if brokerID == "" {
		log.Infof(ctx, "No assigned broker for user %q from cache", req.GetUsername())
		return &proto.GPBResponse{}, nil
	}

	// Updates manager memory to stop needing to query the database for the broker.
	if err = s.brokerManager.SetDefaultBrokerForUser(brokerID, req.GetUsername()); err != nil {
		log.Warningf(ctx, "Last broker used by %q is not available, letting the user selecting one: %v", req.GetUsername(), err)
		return &proto.GPBResponse{}, nil
	}

	return &proto.GPBResponse{
		PreviousBroker: brokerID,
	}, nil
}

// SelectBroker starts a new session and selects the requested broker for the user.
func (s Service) SelectBroker(ctx context.Context, req *proto.SBRequest) (resp *proto.SBResponse, err error) {
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
	case proto.SessionMode_AUTH:
		mode = auth.SessionModeAuth
	case proto.SessionMode_PASSWD:
		mode = auth.SessionModePasswd
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid session mode")
	}

	// Create a session and Memorize selected broker for it.
	sessionID, encryptionKey, err := s.brokerManager.NewSession(brokerID, username, lang, mode)
	if err != nil {
		return nil, err
	}

	return &proto.SBResponse{
		SessionId:     sessionID,
		EncryptionKey: encryptionKey,
	}, err
}

// GetAuthenticationModes fetches a list of authentication modes supported by the broker depending on the session information.
func (s Service) GetAuthenticationModes(ctx context.Context, req *proto.GAMRequest) (resp *proto.GAMResponse, err error) {
	defer decorate.OnError(&err, "could not get authentication modes")

	sessionID := req.GetSessionId()
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "no session ID provided")
	}

	broker, err := s.brokerManager.BrokerFromSessionID(sessionID)
	if err != nil {
		return nil, err
	}

	var supportedLayouts []map[string]string
	for _, layoutPb := range req.GetSupportedUiLayouts() {
		layoutMap, err := layoutPb.ToMap()
		if err != nil {
			return nil, err
		}

		supportedLayouts = append(supportedLayouts, layoutMap)
	}

	authenticationModes, err := broker.GetAuthenticationModes(ctx, sessionID, supportedLayouts)
	if err != nil {
		return nil, err
	}

	var authModesPb []*proto.GAMResponse_AuthenticationMode
	for _, authModeMap := range authenticationModes {
		authModePb, err := proto.AuthModeFromMap(authModeMap)
		if err != nil {
			return nil, err
		}

		authModesPb = append(authModesPb, authModePb)
	}

	return &proto.GAMResponse{
		AuthenticationModes: authModesPb,
	}, nil
}

// SelectAuthenticationMode set given authentication mode as selected for this sessionID to the broker.
func (s Service) SelectAuthenticationMode(ctx context.Context, req *proto.SAMRequest) (resp *proto.SAMResponse, err error) {
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

	uiLayout, err := layouts.NewUIFromMap(uiLayoutInfo)
	if err != nil {
		return nil, err
	}

	return &proto.SAMResponse{UiLayoutInfo: uiLayout.UILayout}, nil
}

// IsAuthenticated returns broker answer to authentication request.
func (s Service) IsAuthenticated(ctx context.Context, req *proto.IARequest) (resp *proto.IAResponse, err error) {
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

	log.Debugf(ctx, "%s: Authentication result: %s", sessionID, access)

	if access != auth.Granted {
		return &proto.IAResponse{
			Access: access,
			Msg:    data,
		}, nil
	}

	var uInfo users.UserInfo
	if err := json.Unmarshal([]byte(data), &uInfo); err != nil {
		return nil, fmt.Errorf("user data from broker invalid: %v", err)
	}

	// Update database and local groups on granted auth.
	if err := s.userManager.UpdateUser(uInfo); err != nil {
		return nil, err
	}

	return &proto.IAResponse{
		Access: access,
		Msg:    "",
	}, nil
}

// SetDefaultBrokerForUser sets the default broker for the given user.
func (s Service) SetDefaultBrokerForUser(ctx context.Context, req *proto.SDBFURequest) (empty *proto.Empty, err error) {
	defer decorate.OnError(&err, "can't set default broker %q for user %q", req.GetBrokerId(), req.GetUsername())

	if req.GetUsername() == "" {
		return nil, status.Error(codes.InvalidArgument, "no user name given")
	}

	if err = s.brokerManager.SetDefaultBrokerForUser(req.GetBrokerId(), req.GetUsername()); err != nil {
		return &proto.Empty{}, err
	}

	if req.GetBrokerId() == brokers.LocalBrokerName {
		return &proto.Empty{}, nil
	}

	if err = s.userManager.UpdateBrokerForUser(req.GetUsername(), req.GetBrokerId()); err != nil {
		return &proto.Empty{}, err
	}

	return &proto.Empty{}, nil
}

// EndSession asks the broker associated with the sessionID to end the session.
func (s Service) EndSession(ctx context.Context, req *proto.ESRequest) (empty *proto.Empty, err error) {
	defer decorate.OnError(&err, "could not abort session")

	sessionID := req.GetSessionId()
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "no session id given")
	}

	return &proto.Empty{}, s.brokerManager.EndSession(sessionID)
}

func uiLayoutToMap(layout *proto.UILayout) (mapLayout map[string]string, err error) {
	m, err := layouts.UILayout{UILayout: layout}.ToMap()
	if err != nil {
		return nil, err
	}

	if layout.RendersQrcode == nil {
		// If the field is not set, we keep retro-compatibility with what
		// we were doing before of the addition of the field.
		m[layouts.RendersQrCode] = layouts.True
	}
	return m, nil
}
