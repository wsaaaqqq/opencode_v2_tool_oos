package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

const bulkOpenConfirmLimit = 5

type sessionBatch struct {
	action    string // open or delete
	targets   []Session
	done      int
	succeeded int
	failed    map[string]bool
	firstErr  string
}

type batchItemDoneMsg struct {
	action string
	id     string
	err    error
}

func (m *model) cancelConfirmation() {
	m.pendingAction = ""
	m.pendingTargets = nil
}

func sameTargets(a, b []Session) bool {
	if len(a) != len(b) {
		return false
	}
	ids := make(map[string]bool, len(a))
	for _, s := range a {
		ids[s.ID] = true
	}
	for _, s := range b {
		if !ids[s.ID] {
			return false
		}
	}
	return true
}

func (m *model) requestAction(action string) tea.Cmd {
	targets := m.actionTargets()
	if len(targets) == 0 {
		m.cancelConfirmation()
		return nil
	}
	needsConfirm := action == "delete" || len(targets) > bulkOpenConfirmLimit
	if needsConfirm {
		if m.pendingAction != action || !sameTargets(targets, m.pendingTargets) {
			// Freeze the set being confirmed, not a slice into a changing list.
			m.pendingAction = action
			m.pendingTargets = append([]Session(nil), targets...)
			return nil
		}
		targets = m.pendingTargets
	}
	m.cancelConfirmation()
	m.batch = &sessionBatch{
		action: action, targets: append([]Session(nil), targets...),
		failed: make(map[string]bool),
	}
	return nextBatchItemCmd(m.batch)
}

// Schedule one item at a time, returning through Update after each item for
// progress and failure handling. Commands never read the mutable model.
func nextBatchItemCmd(batch *sessionBatch) tea.Cmd {
	s := batch.targets[batch.done]
	run := openSessionBg
	if batch.action == "delete" {
		run = func(s Session) error { return DeleteSession(s.ID, s.Directory) }
	}
	return batchItemCmd(batch.action, s, run)
}

func batchItemCmd(action string, s Session, run func(Session) error) tea.Cmd {
	return func() tea.Msg {
		return batchItemDoneMsg{action: action, id: s.ID, err: run(s)}
	}
}

func (m *model) finishBatchItem(msg batchItemDoneMsg) tea.Cmd {
	b := m.batch
	if b == nil || msg.action != b.action || msg.id != b.targets[b.done].ID {
		return nil
	}
	if msg.err != nil {
		b.failed[msg.id] = true
		if b.firstErr == "" {
			b.firstErr = fmt.Sprintf("%s: %v", msg.id, msg.err)
		}
	} else {
		b.succeeded++
		delete(m.markedIDs, msg.id)
		if b.action == "delete" {
			m.removeSession(msg.id)
		}
	}
	b.done++
	if b.done < len(b.targets) {
		return nextBatchItemCmd(b)
	}

	verb := "Opened"
	if b.action == "delete" {
		verb = "Deleted"
	}
	m.notice = fmt.Sprintf("%s: %d succeeded, %d failed", verb, b.succeeded, len(b.failed))
	m.noticeOK = len(b.failed) == 0
	m.clearSelection()
	if len(b.failed) > 0 {
		m.markedIDs = b.failed
		m.notice += "; " + b.firstErr
		// Keep failed items selected for retry; no fallback to the cursor.
		m.refilter(m.focusedID())
	}
	m.batch = nil
	return nil
}

func (m model) batchStatus() string {
	b := m.batch
	verb := "Opening"
	if b.action == "delete" {
		verb = "Deleting"
	}
	return fmt.Sprintf("%s %d/%d (%d succeeded, %d failed)",
		verb, b.done+1, len(b.targets), b.succeeded, len(b.failed))
}
