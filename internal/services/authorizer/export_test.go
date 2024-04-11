package authorizer

type PeerCredsInfo = peerCredsInfo

//nolint:revive // This is a false positive as we returned a typed alias and not the private type.
func NewTestPeerCredsInfo(uid uint32, pid int32) PeerCredsInfo {
	return PeerCredsInfo{uid: uid, pid: pid}
}

var (
	CurrentUserUID        = currentUserUID
	WithCurrentUserAsRoot = withCurrentUserAsRoot
)
