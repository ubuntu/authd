package users

import (
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/types"
)

func (m *Manager) DB() *db.Manager {
	return m.db
}

func (m *Manager) GetOldUserInfoFromDB(name string) (oldUserInfo *types.UserInfo, err error) {
	return m.getOldUserInfoFromDB(name)
}

func CompareNewUserInfoWithUserInfoFromDB(newUserInfo, dbUserInfo types.UserInfo) bool {
	return compareNewUserInfoWithUserInfoFromDB(newUserInfo, dbUserInfo)
}
