package examplebroker

import "sync"

type userInfoBroker struct {
	Password string
}

var (
	exampleUsersMu = sync.RWMutex{}
	exampleUsers   = map[string]userInfoBroker{
		"user1":               {Password: "goodpass"},
		"user2":               {Password: "goodpass"},
		"user3":               {Password: "goodpass"},
		"user-mfa":            {Password: "goodpass"},
		"user-mfa-with-reset": {Password: "goodpass"},
		"user-needs-reset":    {Password: "goodpass"},
		"user-needs-reset2":   {Password: "goodpass"},
		"user-can-reset":      {Password: "goodpass"},
		"user-can-reset2":     {Password: "goodpass"},
		"user-local-groups":   {Password: "goodpass"},
		"user-pre-check":      {Password: "goodpass"},
		"user-sudo":           {Password: "goodpass"},
	}
)
