package himmelblau

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenAcquisitionErrorClassification(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		errorCodes []uint32
		want       error
		wantCode   string
	}{
		"Device_disabled":      {errorCodes: []uint32{deviceDisabledErrorCode}, want: ErrDeviceDisabled, wantCode: "AADSTS135011"},
		"Invalid_redirect_URI": {errorCodes: []uint32{invalidRedirectURIErrorCode}, want: ErrInvalidRedirectURI, wantCode: "AADSTS50011"},
		// AADSTS500113 is a separate documented code, but the same
		// reply-address misconfiguration family as AADSTS50011. It must be
		// classified explicitly rather than caught by prefix matching.
		"No_reply_address":             {errorCodes: []uint32{noReplyAddressErrorCode}, want: ErrInvalidRedirectURI, wantCode: "AADSTS500113"},
		"Missing_client_credentials":   {errorCodes: []uint32{missingClientCredentialsErrorCode}, want: ErrMissingClientCredentials, wantCode: "AADSTS7000218"},
		"Device_authentication_failed": {errorCodes: []uint32{deviceAuthenticationFailedErrorCode}, want: ErrDeviceAuthenticationFailed, wantCode: "AADSTS50155"},
		// Codes match exactly. A code that only starts with a handled one, such
		// as 501559 for 50155, must take the non-destructive catch-all and never
		// clear the cached device registration.
		"Unrelated_code_sharing_50155_prefix_is_not_device_authentication_failed": {errorCodes: []uint32{501559}},
		// A genuinely unclassified/unknown AADSTS code (not one of the ones
		// above) must fall back to a plain error, never to one of the specific
		// sentinels.
		"Unexpected_token_acquisition": {errorCodes: []uint32{999999}},
		"Empty_error_codes_list":       {errorCodes: nil},
		// libhimmelblau can report a list of codes. The classification must keep
		// scanning past codes it does not know, otherwise a documented signal
		// behind an unrecognized code (for example the device-authentication
		// failure that starts device recovery) is lost.
		"Known_code_after_unknown_code": {
			errorCodes: []uint32{999999, deviceAuthenticationFailedErrorCode},
			want:       ErrDeviceAuthenticationFailed,
			wantCode:   "AADSTS50155",
		},
		"Missing_client_credentials_after_unknown_code": {
			errorCodes: []uint32{999999, missingClientCredentialsErrorCode},
			want:       ErrMissingClientCredentials,
			wantCode:   "AADSTS7000218",
		},
		// Two recognized codes that do not deny the login: the one
		// libhimmelblau reports first classifies the failure, and both stay
		// visible in the message. The reverse case below pins the other order.
		"First_classified_code_wins": {
			errorCodes: []uint32{missingClientCredentialsErrorCode, deviceAuthenticationFailedErrorCode},
			want:       ErrMissingClientCredentials,
			wantCode:   "AADSTS7000218",
		},
		"First_classified_code_wins_reversed": {
			errorCodes: []uint32{deviceAuthenticationFailedErrorCode, missingClientCredentialsErrorCode},
			want:       ErrDeviceAuthenticationFailed,
			wantCode:   "AADSTS50155",
		},
		// A code that denies the login wins over one that keeps the cached
		// state, even when that one comes first. Else a returning login would
		// use cached groups and never record the disabled device.
		"Device_disabled_wins_after_missing_client_credentials": {
			errorCodes: []uint32{missingClientCredentialsErrorCode, deviceDisabledErrorCode},
			want:       ErrDeviceDisabled,
			wantCode:   "AADSTS135011",
		},
		"Invalid_redirect_URI_wins_after_missing_client_credentials": {
			errorCodes: []uint32{missingClientCredentialsErrorCode, invalidRedirectURIErrorCode},
			want:       ErrInvalidRedirectURI,
			wantCode:   "AADSTS50011",
		},
		"Device_disabled_wins_after_device_authentication_failed": {
			errorCodes: []uint32{deviceAuthenticationFailedErrorCode, deviceDisabledErrorCode},
			want:       ErrDeviceDisabled,
			wantCode:   "AADSTS135011",
		},
		"Device_disabled_wins_over_invalid_redirect_URI": {
			errorCodes: []uint32{invalidRedirectURIErrorCode, deviceDisabledErrorCode},
			want:       ErrDeviceDisabled,
			wantCode:   "AADSTS135011",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tokenAcquisitionError(tc.errorCodes, "token acquisition failed")
			require.Contains(t, err.Error(), "token acquisition failed",
				"the provider message must survive into the returned error")
			// Every reported code must stay visible, also in a classified
			// failure: libhimmelblau can report codes only through its list, so
			// a code missing from the error is missing from the log too.
			for _, code := range tc.errorCodes {
				require.Contains(t, err.Error(), fmt.Sprintf("%d", code),
					"every reported AADSTS code must stay visible in the error message")
			}
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				require.Contains(t, err.Error(), tc.wantCode)
				return
			}
			require.Error(t, err)
			for _, sentinel := range []error{ErrDeviceDisabled, ErrInvalidRedirectURI, ErrMissingClientCredentials, ErrDeviceAuthenticationFailed} {
				require.NotErrorIs(t, err, sentinel)
			}
		})
	}
}
