package adapter

import (
	"context"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/authd/pam/internal/proto"
)

func TestDebugMessageFormatter(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		msg tea.Msg

		wantSafeString  string
		wantDebugString string
	}{
		"Empty_msg": {},
		"StageChanged_message": {
			msg:            StageChanged{proto.Stage_brokerSelection},
			wantSafeString: `adapter.StageChanged{Stage:"brokerSelection"}`,
		},
		"ChangeStage_message": {
			msg:            ChangeStage{Stage: proto.Stage_authModeSelection},
			wantSafeString: `adapter.ChangeStage{Stage:"authModeSelection"}`,
		},
		"nativeStageChangeRequest_message": {
			msg:            ChangeStage{Stage: proto.Stage_authModeSelection},
			wantSafeString: `adapter.ChangeStage{Stage:"authModeSelection"}`,
		},
		"New_password_check": {
			msg:             newPasswordCheck{password: "Super secret password!"},
			wantSafeString:  `adapter.newPasswordCheck{ctx:context.Context(nil), password:"***********"}`,
			wantDebugString: `adapter.newPasswordCheck{ctx:context.Context(nil), password:"Super secret password!"}`,
		},
		"New_password_check_result": {
			msg:             newPasswordCheckResult{password: "Super secret password!", msg: "Some message"},
			wantSafeString:  `adapter.newPasswordCheckResult{ctx:context.Context(nil), password:"***********", msg:"Some message"}`,
			wantDebugString: `adapter.newPasswordCheckResult{ctx:context.Context(nil), password:"Super secret password!", msg:"Some message"}`,
		},
		"Key_rune_message": {
			msg:             tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p', 'a', 's', 's'}},
			wantSafeString:  ``,
			wantDebugString: `tea.KeyMsg{pass}`,
		},
		"Key_modifier_message": {
			msg:            tea.KeyMsg{Type: tea.KeyTab, Runes: []rune{'p', 'a', 's', 's'}},
			wantSafeString: `tea.KeyMsg{tab}`,
		},
		"isAuthenticatedRequested_empty": {
			msg:            isAuthenticatedRequested{},
			wantSafeString: `adapter.isAuthenticatedRequested{<nil>{}}`,
		},
		"isAuthenticatedRequested_with_secret": {
			msg: isAuthenticatedRequested{
				item: &authd.IARequest_AuthenticationData_Secret{Secret: "super-secret!"},
			},
			wantSafeString:  `adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Secret{Secret:"***********"}}`,
			wantDebugString: `adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Secret{Secret:"super-secret!"}}`,
		},
		"isAuthenticatedRequested_with_wait": {
			msg: isAuthenticatedRequested{
				item: &authd.IARequest_AuthenticationData_Wait{Wait: "wait!"},
			},
			wantSafeString: `adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Wait{Wait:"wait!"}}`,
		},
		"isAuthenticatedRequested_with_skip": {
			msg: isAuthenticatedRequested{
				item: &authd.IARequest_AuthenticationData_Skip{Skip: "skip!"},
			},
			wantSafeString: `adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Skip{Skip:"skip!"}}`,
		},
		"isAuthenticatedRequestedSend_empty": {
			msg:            isAuthenticatedRequestedSend{},
			wantSafeString: `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{<nil>{}}}`,
		},
		"isAuthenticatedRequestedSend_with_secret": {
			msg: isAuthenticatedRequestedSend{
				isAuthenticatedRequested: isAuthenticatedRequested{
					item: &authd.IARequest_AuthenticationData_Secret{Secret: "super-secret!"},
				},
			},
			wantSafeString:  `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Secret{Secret:"***********"}}}`,
			wantDebugString: `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Secret{Secret:"super-secret!"}}}`,
		},
		"isAuthenticatedRequestedSend_with_wait": {
			msg: isAuthenticatedRequestedSend{
				isAuthenticatedRequested: isAuthenticatedRequested{
					item: &authd.IARequest_AuthenticationData_Wait{Wait: "wait!"},
				},
			},
			wantSafeString: `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Wait{Wait:"wait!"}}}`,
		},
		"isAuthenticatedRequestedSend_with_skip": {
			msg: isAuthenticatedRequestedSend{
				isAuthenticatedRequested: isAuthenticatedRequested{
					item: &authd.IARequest_AuthenticationData_Skip{Skip: "skip!"},
				},
			},
			wantSafeString: `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Skip{Skip:"skip!"}}}`,
		},
		"brokerInfo": {
			msg:            &authd.ABResponse_BrokerInfo{Id: "broker-id", Name: "broker-name", BrandIcon: nil},
			wantSafeString: `*authd.ABResponse_BrokerInfo{{"id":"broker-id","name":"broker-name"}}`,
		},
		"brokerInfo_slice": {
			msg:            []*authd.ABResponse_BrokerInfo{{Id: "broker-id", Name: "broker-name", BrandIcon: nil}},
			wantSafeString: `[]*authd.ABResponse_BrokerInfo{[{"id":"broker-id","name":"broker-name"}]}`,
		},
		"brokersListReceived_empty": {
			msg:            brokersListReceived{},
			wantSafeString: `adapter.brokersListReceived{brokers:[]*authd.ABResponse_BrokerInfo{null}}`,
		},
		"brokersListReceived_with_brokers": {
			msg: brokersListReceived{[]*authd.ABResponse_BrokerInfo{{
				Id: "broker-id", Name: "broker-name", BrandIcon: nil,
			}}},
			wantSafeString: `adapter.brokersListReceived{brokers:[]*authd.ABResponse_BrokerInfo{[{"id":"broker-id","name":"broker-name"}]}}`,
		},
		"UILayout": {
			msg: &authd.UILayout{
				Type:    "Type",
				Label:   ptrValue("Label"),
				Content: ptrValue("Content"),
				Wait:    ptrValue("Wait"),
				Button:  ptrValue("Button"),
				Code:    ptrValue("Code"),
				Entry:   ptrValue("Entry"),
			},
			wantSafeString: `*authd.UILayout{{"type":"Type","label":"Label","button":"Button","wait":"Wait","entry":"Entry","content":"Content","code":"Code"}}`,
		},
		"UILayout_slice": {
			msg: []*authd.UILayout{{
				Type:    "Type",
				Label:   ptrValue("Label"),
				Content: ptrValue("Content"),
				Wait:    ptrValue("Wait"),
				Button:  ptrValue("Button"),
				Code:    ptrValue("Code"),
				Entry:   ptrValue("Entry"),
			}},
			wantSafeString: `[]*authd.UILayout{[{"type":"Type","label":"Label","button":"Button","wait":"Wait","entry":"Entry","content":"Content","code":"Code"}]}`,
		},
		"UILayoutReceived": {
			msg:            UILayoutReceived{&authd.UILayout{Type: "Foo"}},
			wantSafeString: `adapter.UILayoutReceived{layouts:*authd.UILayout{{"type":"Foo"}}}`,
		},
		"supportedUILayoutsReceived": {
			msg: supportedUILayoutsReceived{[]*authd.UILayout{{
				Type:    "Type",
				Label:   ptrValue("Label"),
				Content: ptrValue("Content"),
				Wait:    ptrValue("Wait"),
				Button:  ptrValue("Button"),
				Code:    ptrValue("Code"),
				Entry:   ptrValue("Entry"),
			}, {Type: "Other"}}},
			wantSafeString: `adapter.supportedUILayoutsReceived{layouts:[]*authd.UILayout{[{"type":"Type","label":"Label","button":"Button","wait":"Wait","entry":"Entry","content":"Content","code":"Code"},{"type":"Other"}]}}`,
		},
		"GAMResponse_AuthenticationMode": {
			msg:            &authd.GAMResponse_AuthenticationMode{Id: "id", Label: "Label"},
			wantSafeString: `*authd.GAMResponse_AuthenticationMode{{"id":"id","label":"Label"}}`,
		},
		"GAMResponse_AuthenticationMode_slice": {
			msg:            []*authd.GAMResponse_AuthenticationMode{{Id: "id", Label: "Label"}},
			wantSafeString: `[]*authd.GAMResponse_AuthenticationMode{[{"id":"id","label":"Label"}]}`,
		},
		"authModesReceived": {
			msg: authModesReceived{
				authModes: []*authd.GAMResponse_AuthenticationMode{
					{Id: "id1", Label: "Label1"},
					{Id: "id2", Label: "Label2"},
				},
			},
			wantSafeString: `adapter.authModesReceived{authModes:[]*authd.GAMResponse_AuthenticationMode{[{"id":"id1","label":"Label1"},{"id":"id2","label":"Label2"}]}}`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.wantSafeString, defaultSafeMessageFormatter(tc.msg),
				"MessageFormatter safe result mismatches expected")

			if tc.wantDebugString == "" {
				tc.wantDebugString = tc.wantSafeString
			}

			require.Equal(t, tc.wantDebugString, debugMessageFormatter(tc.msg),
				"MessageFormatter debug result mismatches expected")
		})
	}
}

func TestSafeMessageDebug(t *testing.T) {
	// This cannot be parallel

	tests := map[string]struct {
		prefix        string
		msg           tea.Msg
		formatAndArgs []any

		wantSafeString  string
		wantDebugString string
	}{
		"Empty_msg": {},
		"Empty_msg_with_prefix": {
			prefix: "prefix",
		},
		"Empty_msg_with_prefix_and_suffix": {
			prefix:        "prefix",
			formatAndArgs: []any{"suffix"},
		},
		"StageChanged_message": {
			msg:            StageChanged{proto.Stage_brokerSelection},
			wantSafeString: `adapter.StageChanged{Stage:"brokerSelection"}`,
		},
		"startAuthentication_message_with_prefix": {
			msg:            startAuthentication{},
			prefix:         "prefix",
			wantSafeString: "prefix: adapter.startAuthentication{}",
		},
		"startAuthentication_message_with_prefix_and_single_value_suffix": {
			msg:            startAuthentication{},
			prefix:         "prefix",
			formatAndArgs:  []any{true},
			wantSafeString: "prefix: adapter.startAuthentication{}, true",
		},
		"startAuthentication_message_with_prefix_and_multiple_value_suffix": {
			msg:            startAuthentication{},
			prefix:         "prefix",
			formatAndArgs:  []any{true, false},
			wantSafeString: "prefix: adapter.startAuthentication{}, true false",
		},
		"startAuthentication_message_with_prefix_and_multiple_formatted_values_suffix": {
			msg:    startAuthentication{},
			prefix: "prefix",
			formatAndArgs: []any{
				newPasswordCheck{password: "password!"},
				UILayoutReceived{&authd.UILayout{Type: "Foo"}},
				authModesReceived{
					authModes: []*authd.GAMResponse_AuthenticationMode{
						{Id: "id1", Label: "Label1"},
						{Id: "id2", Label: "Label2"},
					},
				},
			},
			wantDebugString: `prefix: adapter.startAuthentication{}, adapter.newPasswordCheck{ctx:context.Context(nil), password:"password!"} adapter.UILayoutReceived{layouts:*authd.UILayout{{"type":"Foo"}}} adapter.authModesReceived{authModes:[]*authd.GAMResponse_AuthenticationMode{[{"id":"id1","label":"Label1"},{"id":"id2","label":"Label2"}]}}`,
			wantSafeString:  `prefix: adapter.startAuthentication{}, adapter.newPasswordCheck{ctx:context.Context(nil), password:"***********"} adapter.UILayoutReceived{layouts:*authd.UILayout{{"type":"Foo"}}} adapter.authModesReceived{authModes:[]*authd.GAMResponse_AuthenticationMode{[{"id":"id1","label":"Label1"},{"id":"id2","label":"Label2"}]}}`,
		},
		"startAuthentication_message_with_prefix_and_single_string_suffix": {
			msg:            startAuthentication{},
			prefix:         "prefix",
			formatAndArgs:  []any{"suffix"},
			wantSafeString: "prefix: adapter.startAuthentication{}, suffix",
		},
		"startAuthentication_message_with_prefix_and_format_suffix": {
			msg:            startAuthentication{},
			prefix:         "prefix",
			formatAndArgs:  []any{"suffix is %#v and %q", stopAuthentication{}, "suffix"},
			wantSafeString: `prefix: adapter.startAuthentication{}, suffix is adapter.stopAuthentication{} and "suffix"`,
		},
		"New_password_check": {
			msg:             newPasswordCheck{password: "Super secret password!"},
			wantSafeString:  `adapter.newPasswordCheck{ctx:context.Context(nil), password:"***********"}`,
			wantDebugString: `adapter.newPasswordCheck{ctx:context.Context(nil), password:"Super secret password!"}`,
		},
		"New_password_check_result": {
			msg:             newPasswordCheckResult{password: "Super secret password!", msg: "Some message"},
			wantSafeString:  `adapter.newPasswordCheckResult{ctx:context.Context(nil), password:"***********", msg:"Some message"}`,
			wantDebugString: `adapter.newPasswordCheckResult{ctx:context.Context(nil), password:"Super secret password!", msg:"Some message"}`,
		},
		"Key_rune_message": {
			msg:             tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p', 'a', 's', 's'}},
			wantSafeString:  ``,
			wantDebugString: `tea.KeyMsg{pass}`,
		},
		"Key_modifier_message": {
			msg:            tea.KeyMsg{Type: tea.KeyTab, Runes: []rune{'p', 'a', 's', 's'}},
			wantSafeString: `tea.KeyMsg{tab}`,
		},
		"isAuthenticatedRequested_empty": {
			msg:            isAuthenticatedRequested{},
			wantSafeString: `adapter.isAuthenticatedRequested{<nil>{}}`,
		},
		"isAuthenticatedRequested_with_secret": {
			msg: isAuthenticatedRequested{
				item: &authd.IARequest_AuthenticationData_Secret{Secret: "super-secret!"},
			},
			wantSafeString:  `adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Secret{Secret:"***********"}}`,
			wantDebugString: `adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Secret{Secret:"super-secret!"}}`,
		},
		"isAuthenticatedRequested_with_wait": {
			msg: isAuthenticatedRequested{
				item: &authd.IARequest_AuthenticationData_Wait{Wait: "wait!"},
			},
			wantSafeString: `adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Wait{Wait:"wait!"}}`,
		},
		"isAuthenticatedRequested_with_skip": {
			msg: isAuthenticatedRequested{
				item: &authd.IARequest_AuthenticationData_Skip{Skip: "skip!"},
			},
			wantSafeString: `adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Skip{Skip:"skip!"}}`,
		},
		"isAuthenticatedRequestedSend_empty": {
			msg:            isAuthenticatedRequestedSend{},
			wantSafeString: `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{<nil>{}}}`,
		},
		"isAuthenticatedRequestedSend_with_secret": {
			msg: isAuthenticatedRequestedSend{
				isAuthenticatedRequested: isAuthenticatedRequested{
					item: &authd.IARequest_AuthenticationData_Secret{Secret: "super-secret!"},
				},
			},
			wantSafeString:  `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Secret{Secret:"***********"}}}`,
			wantDebugString: `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Secret{Secret:"super-secret!"}}}`,
		},
		"isAuthenticatedRequestedSend_with_wait": {
			msg: isAuthenticatedRequestedSend{
				isAuthenticatedRequested: isAuthenticatedRequested{
					item: &authd.IARequest_AuthenticationData_Wait{Wait: "wait!"},
				},
			},
			wantSafeString: `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Wait{Wait:"wait!"}}}`,
		},
		"isAuthenticatedRequestedSend_with_skip": {
			msg: isAuthenticatedRequestedSend{
				isAuthenticatedRequested: isAuthenticatedRequested{
					item: &authd.IARequest_AuthenticationData_Skip{Skip: "skip!"},
				},
			},
			wantSafeString: `adapter.isAuthenticatedRequestedSend{adapter.isAuthenticatedRequested{*authd.IARequest_AuthenticationData_Skip{Skip:"skip!"}}}`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handlerCalled := false
			wantCtx := context.Background()
			log.SetLevelHandler(log.DebugLevel, func(ctx context.Context, l log.Level, format string, args ...interface{}) {
				t.Logf("Format: %q", format)
				t.Log(append([]any{"Args:"}, args...)...)
				handlerCalled = true
				require.Equal(t, wantCtx, ctx, "Context should match expected")
				require.Equal(t, tc.wantSafeString, fmt.Sprintf(format, args...),
					"Format for safe usage should match")
			})

			initialLogLevel := log.GetLevel()
			log.SetLevel(log.DebugLevel)
			t.Cleanup(func() { log.SetLevel(initialLogLevel) })

			initialFormatter := debugMessageFormatter
			debugMessageFormatter = defaultSafeMessageFormatter
			t.Cleanup(func() { debugMessageFormatter = initialFormatter })

			if tc.prefix == "" {
				safeMessageDebug(tc.msg, tc.formatAndArgs...)
			} else {
				safeMessageDebugWithPrefix(tc.prefix, tc.msg, tc.formatAndArgs...)
			}
			require.Equal(t, tc.wantSafeString != "", handlerCalled,
				"Handler should have been called")
		})

		t.Run(fmt.Sprintf("%s_debug_mode", name), func(t *testing.T) {
			if tc.wantDebugString == "" {
				tc.wantDebugString = tc.wantSafeString
			}

			handlerCalled := false
			wantCtx := context.Background()
			log.SetLevelHandler(log.DebugLevel, func(ctx context.Context, l log.Level, format string, args ...interface{}) {
				t.Logf(format, args...)
				handlerCalled = true
				t.Logf("Format: %q", format)
				t.Log(append([]any{"Args:"}, args...)...)
				require.Equal(t, wantCtx, ctx, "Context should match expected")
				require.Equal(t, tc.wantDebugString, fmt.Sprintf(format, args...),
					"Format for debug usage should match")
			})

			initialLogLevel := log.GetLevel()
			log.SetLevel(log.DebugLevel)
			t.Cleanup(func() { log.SetLevel(initialLogLevel) })

			initialFormatter := debugMessageFormatter
			debugMessageFormatter = testMessageFormatter
			t.Cleanup(func() { debugMessageFormatter = initialFormatter })

			if tc.prefix == "" {
				safeMessageDebug(tc.msg, tc.formatAndArgs...)
			} else {
				safeMessageDebugWithPrefix(tc.prefix, tc.msg, tc.formatAndArgs...)
			}
			require.Equal(t, tc.wantDebugString != "", handlerCalled,
				"Handler should have been called")
		})
	}
}

func ptrValue[T any](value T) *T {
	return &value
}
