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

const (
	// UserIntegrationPrefix is the prefix for an user for integration tests.
	UserIntegrationPrefix = "user-integration-"
	// UserIntegrationMfaPrefix is the prefix for an mfa user for integration tests.
	UserIntegrationMfaPrefix = "user-mfa-integration-"
	// UserIntegrationMfaNeedsResetPrefix is the prefix for an mfa-needs-reset user for integration tests.
	UserIntegrationMfaNeedsResetPrefix = "user-mfa-needs-reset-integration-"
	// UserIntegrationMfaWithResetPrefix is the prefix for an mfa-with-reset user for integration tests.
	UserIntegrationMfaWithResetPrefix = "user-mfa-with-reset-integration-"
	// UserIntegrationNeedsResetPrefix is the prefix for a needs-reset user for integration tests.
	UserIntegrationNeedsResetPrefix = "user-needs-reset-integration-"
	// UserIntegrationCanResetPrefix is the prefix for a can-reset user for integration tests.
	UserIntegrationCanResetPrefix = "user-can-reset-integration-"
	// UserIntegrationLocalGroupsPrefix is the prefix for a local-groups user for integration tests.
	UserIntegrationLocalGroupsPrefix = "user-local-groups-integration-"
	// UserIntegrationQrcodeStaticPrefix is the prefix for a static qrcode user for integration tests.
	UserIntegrationQrcodeStaticPrefix = "user-integration-qrcode-static-"
	// UserIntegrationPreCheckPrefix is the prefix for a pre-check user for integration tests.
	UserIntegrationPreCheckPrefix = "user-integration-pre-check-"
	// UserIntegrationUnexistent is an unexistent user leading to a non-existent user error.
	UserIntegrationUnexistent = "user-unexistent"
)