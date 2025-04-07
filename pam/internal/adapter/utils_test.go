package adapter

import (
	"context"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
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
			wantSafeString: "adapter.StageChanged{Stage:1}",
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
			wantSafeString: "adapter.StageChanged{Stage:1}",
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handlerCalled := false
			wantCtx := context.Background()
			log.SetLevelHandler(log.DebugLevel, func(ctx context.Context, l log.Level, format string, args ...interface{}) {
				t.Logf(format, args...)
				handlerCalled = true
				require.Equal(t, wantCtx, ctx, "Context should match expected")
				require.Equal(t, tc.wantSafeString, fmt.Sprintf(format, args...),
					"Format should match")
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
				t.Logf("Called with %q", format)
				t.Logf("Atgs are %#v", args)
				require.Equal(t, wantCtx, ctx, "Context should match expected")
				require.Equal(t, tc.wantDebugString, fmt.Sprintf(format, args...),
					"Format should match")
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
