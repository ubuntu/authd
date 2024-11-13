package adapter

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ubuntu/authd/pam/internal/proto"
)

type gdmModeler interface {
	tea.Model
	changeStage(proto.Stage) tea.Cmd
	stopConversations() gdmModeler
	conversationsActive() bool
}
