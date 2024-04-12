package authorizer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/services/authorizer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func TestNew(t *testing.T) {
	t.Parallel()

	a := authorizer.New()

	require.NotNil(t, a, "New authorizer is created")
}

func TestIsRequestFromRoot(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		currentUserNotRoot bool
		noPeerCredsInfo    bool
		noAuthInfo         bool

		wantErr bool
	}{
		"Granted if current user considered as root": {},

		"Error as deny when current user is not root": {currentUserNotRoot: true, wantErr: true},
		"Error as deny when missing peer creds Info":  {noPeerCredsInfo: true, wantErr: true},
		"Error as deny when missing auth info creds":  {noAuthInfo: true, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Setup peer creds info
			ctx := context.Background()
			if !tc.noPeerCredsInfo {
				var authInfo credentials.AuthInfo
				if !tc.noAuthInfo {
					uid := authorizer.CurrentUserUID()
					authInfo = authorizer.NewTestPeerCredsInfo(uid, int32(uid))
				}
				p := peer.Peer{
					AuthInfo: authInfo,
				}
				ctx = peer.NewContext(ctx, &p)
			}

			var opts []authorizer.Option
			if !tc.currentUserNotRoot {
				opts = append(opts, authorizer.WithCurrentUserAsRoot())
			}
			a := authorizer.New(opts...)

			err := a.IsRequestFromRoot(ctx)

			if tc.wantErr {
				require.Error(t, err, "IsRequestFromRoot should deny access but didn't")
				return
			}
			require.NoError(t, err, "IsRequestFromRoot should allow access but didn't")
		})
	}
}

func TestWithUnixPeerCreds(t *testing.T) {
	t.Parallel()

	g := grpc.NewServer(authorizer.WithUnixPeerCreds())

	require.NotNil(t, g, "New grpc with Unix Peer Creds is created")
}
