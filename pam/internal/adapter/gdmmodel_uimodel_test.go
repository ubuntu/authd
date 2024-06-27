package adapter

import (
	"context"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/pam/internal/gdm"
	"github.com/ubuntu/authd/pam/internal/gdm_test"
	"github.com/ubuntu/authd/pam/internal/proto"
)

// gdmTestUIModel is an override of [UIModel] used for testing the module with gdm.
type gdmTestUIModel struct {
	UIModel

	mu sync.Mutex

	gdmHandler *gdmConvHandler

	wantMessages        []tea.Msg
	wantMessagesHandled chan struct{}

	program           *tea.Program
	programShouldQuit atomic.Bool
}

// Custom messages for testing the gdm model.

type gdmTestAddPollResultEvent struct {
	event *gdm.EventData
}

type gdmTestWaitForCommandsDone struct {
	seq int64
}

type gdmTestWaitForStage struct {
	stage    proto.Stage
	events   []*gdm.EventData
	commands []tea.Cmd
}

type gdmTestWaitForStageDone gdmTestWaitForStage

type gdmTestSendAuthDataWhenReady struct {
	item authd.IARequestAuthenticationDataItem
}

func (m *gdmTestUIModel) maybeHandleWantMessageUnlocked(msg tea.Msg) {
	returnErrorMsg, isError := msg.(PamReturnError)

	idx := slices.IndexFunc(m.wantMessages, func(wm tea.Msg) bool {
		match := reflect.DeepEqual(wm, msg)
		if match {
			return true
		}
		if !isError {
			return false
		}
		pamErr, ok := wm.(PamReturnError)
		if !ok {
			return false
		}
		if pamErr.Message() != gdmTestIgnoredMessage {
			return false
		}
		return pamErr.Status() == returnErrorMsg.Status()
	})

	if idx < 0 {
		return
	}

	m.wantMessages = slices.Delete(m.wantMessages, idx, idx+1)
	if len(m.wantMessages) == 0 {
		close(m.wantMessagesHandled)
	}
}

func (m *gdmTestUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	log.Debugf(context.TODO(), "%#v", msg)

	m.mu.Lock()
	defer m.mu.Unlock()

	commands := []tea.Cmd{}

	_, cmd := m.UIModel.Update(msg)
	commands = append(commands, cmd)

	switch msg := msg.(type) {
	case gdmTestAddPollResultEvent:
		m.gdmHandler.appendPollResultEvents(msg.event)

	case gdmTestWaitForStage:
		doneMsg := (*gdmTestWaitForStageDone)(&msg)
		if len(doneMsg.commands) > 0 {
			seq := gdmTestSequentialMessages.Add(1)
			doneCommandsMsg := gdmTestWaitForCommandsDone{seq: seq}
			doneMsg.commands = append(doneMsg.commands, sendEvent(doneCommandsMsg))
			m.wantMessages = append(m.wantMessages, doneCommandsMsg)
		}

		waitFunc := m.gdmHandler.waitForStageChange(msg.stage)
		if waitFunc == nil {
			commands = append(commands, sendEvent(doneMsg))
			break
		}

		m.wantMessages = append(m.wantMessages, doneMsg)

		go func() {
			log.Debugf(context.TODO(), "Waiting for stage reached: %#v", doneMsg)
			waitFunc()
			log.Debugf(context.TODO(), "Stage reached: %#v", doneMsg)

			m.program.Send(doneMsg)
		}()

	case *gdmTestWaitForStageDone:
		msgCommands := tea.Sequence(msg.commands...)
		if len(msg.events) > 0 {
			m.gdmHandler.appendPollResultEvents(msg.events...)
			// If we've events as poll results, let's wait for a polling cycle to complete
			msgCommands = tea.Sequence(tea.Tick(gdmPollFrequency, func(t time.Time) tea.Msg {
				return nil
			}), msgCommands)
		}
		commands = append(commands, msgCommands)

	case gdmTestSendAuthDataWhenReady:
		doneMsg := gdmTestWaitForCommandsDone{seq: gdmTestSequentialMessages.Add(1)}
		m.wantMessages = append(m.wantMessages, doneMsg)

		go func() {
			m.gdmHandler.waitForAuthenticationStarted()
			if msg.item != nil {
				m.gdmHandler.appendPollResultEvents(gdm_test.IsAuthenticatedEvent(msg.item))
			}
			m.program.Send(tea.Sequence(tea.Tick(gdmPollFrequency, func(t time.Time) tea.Msg {
				return sendEvent(doneMsg)
			}), sendEvent(doneMsg))())
		}()

	case gdmTestWaitForCommandsDone:
		log.Debugf(context.TODO(), "Sequential messages done: %v", msg.seq)

	case isAuthenticatedCancelled:
		m.gdmHandler.consumeAuthenticationStartedEvents()
	}

	m.maybeHandleWantMessageUnlocked(msg)
	return m, tea.Sequence(commands...)
}

func (m *gdmTestUIModel) filterFunc(model tea.Model, msg tea.Msg) tea.Msg {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := msg.(tea.QuitMsg); ok {
		// Quit is never sent to the Update func so we handle it earlier.
		m.maybeHandleWantMessageUnlocked(msg)
		if !m.programShouldQuit.Load() {
			return nil
		}
	}

	return m.MsgFilter(model, msg)
}
