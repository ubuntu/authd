// Package himmelblau provides functions to use the libhimmelblau library.
package himmelblau

import (
	"errors"
	"fmt"
	"strings"
)

// ErrDeviceDisabled is returned when the device is disabled in Microsoft Entra ID.
var ErrDeviceDisabled = fmt.Errorf("device is disabled in Microsoft Entra ID")

// ErrInvalidRedirectURI is returned when the redirect URI of the client application is missing or invalid.
var ErrInvalidRedirectURI = fmt.Errorf("invalid redirect URI")

// ErrMissingClientCredentials is returned when the token endpoint requires
// client credentials but none were supplied.
var ErrMissingClientCredentials = fmt.Errorf("token endpoint requires client credentials")

// ErrDeviceAuthenticationFailed is returned when Microsoft Entra reports that
// the device itself failed authentication (AADSTS50155). Entra returns this
// code both when the device object is gone (for example after an administrator
// deleted it) and, transiently, while a newly registered device has not been
// replicated yet. Callers must therefore retry a fresh registration for a
// bounded time before treating the failure as final, like himmelblau-idm does
// (see the DEVICE_AUTH_FAIL retry in unix_user_online_auth_step,
// src/common/src/idprovider/himmelblau.rs).
var ErrDeviceAuthenticationFailed = fmt.Errorf("device authentication failed in Microsoft Entra ID")

// Entra AADSTS error codes as defined in
// https://learn.microsoft.com/en-us/entra/identity-platform/reference-error-codes
const (
	// AADSTS135011 Device used during the authentication is disabled.
	deviceDisabledErrorCode = 135011
	// AADSTS50011 InvalidReplyTo - The reply address is missing, misconfigured,
	// or doesn't match reply addresses configured for the app.
	invalidRedirectURIErrorCode = 50011
	// AADSTS500113 - No reply address is registered for the application.
	// A separate documented code, but the same reply-address misconfiguration
	// family as AADSTS50011.
	noReplyAddressErrorCode = 500113
	// AADSTS7000218 RequestBodyMustContainClientAssertion - The request body
	// must contain a client_assertion or client_secret.
	missingClientCredentialsErrorCode = 7000218
	// AADSTS50155 DeviceAuthenticationFailed; see ErrDeviceAuthenticationFailed.
	deviceAuthenticationFailedErrorCode = 50155
)

// errorCodeSentinels maps each classified AADSTS code to its sentinel error.
var errorCodeSentinels = map[uint32]error{
	invalidRedirectURIErrorCode:         ErrInvalidRedirectURI,
	noReplyAddressErrorCode:             ErrInvalidRedirectURI,
	missingClientCredentialsErrorCode:   ErrMissingClientCredentials,
	deviceDisabledErrorCode:             ErrDeviceDisabled,
	deviceAuthenticationFailedErrorCode: ErrDeviceAuthenticationFailed,
}

// aadstsCodes formats the codes the way Microsoft documents them, for example
// "AADSTS500113", so the log shows the whole code and not only the prefix
// that matched.
func aadstsCodes(errorCodes []uint32) string {
	codes := make([]string, len(errorCodes))
	for i, errorCode := range errorCodes {
		codes[i] = fmt.Sprintf("AADSTS%d", errorCode)
	}
	return strings.Join(codes, ", ")
}

func tokenAcquisitionError(errorCodes []uint32, message string) error {
	// Match codes exactly: a wrong guess is destructive (50155 re-enrolls the
	// device, 135011 and 50011 deny the login). An unknown code takes the
	// catch-all below, which keeps the cached state.
	//
	// A code that denies the login wins over every other code in the list,
	// whatever the order. Otherwise a code that keeps the cached state could
	// hide it. A disabled device comes first, so its state is recorded.
	for _, terminal := range []error{ErrDeviceDisabled, ErrInvalidRedirectURI} {
		for _, errorCode := range errorCodes {
			if errors.Is(errorCodeSentinels[errorCode], terminal) {
				return fmt.Errorf("%w (%s): %s", terminal, aadstsCodes(errorCodes), message)
			}
		}
	}
	for _, errorCode := range errorCodes {
		if sentinel, ok := errorCodeSentinels[errorCode]; ok {
			return fmt.Errorf("%w (%s): %s", sentinel, aadstsCodes(errorCodes), message)
		}
	}
	return fmt.Errorf("error acquiring access token using refresh token (%s): %v", aadstsCodes(errorCodes), message)
}
