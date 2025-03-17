package permissions

type PeerCredsInfo = peerCredsInfo

func NewTestPeerCredsInfo(uid uint32, pid int32) PeerCredsInfo {
	return PeerCredsInfo{uid: uid, pid: pid}
}

var (
	CurrentUserUID = currentUserUID
)
