package users

import (
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/types"
)

const (
	MinAllowedUID  = minAllowedUID
	MinAllowedGID  = minAllowedGID
	MaxSuggestedID = maxSuggestedID
)

func (m *Manager) DB() *db.Manager {
	return m.db
}

func (m *Manager) RealIDGenerator() *IDGenerator {
	//nolint:forcetypeassert  // We really want to panic if it's not true.
	return m.idGenerator.(*IDGenerator)
}

func (m *Manager) GetOldUserInfoFromDB(name string) (oldUserInfo *types.UserInfo, err error) {
	return m.getOldUserInfoFromDB(name)
}

func CompareNewUserInfoWithUserInfoFromDB(newUserInfo, dbUserInfo types.UserInfo) bool {
	return compareNewUserInfoWithUserInfoFromDB(newUserInfo, dbUserInfo)
}
