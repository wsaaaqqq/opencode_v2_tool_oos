package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// These tests drive Update directly. They never execute real open/delete/copy
// commands and do not need access to an OpenCode database or Windows Terminal.
func selectionModel(n, cursor int) model {
	m := initialModel("unused-test.db", "")
	m.ready, m.sessionsDone, m.msgsDone = true, true, true
	m.width, m.height = 120, 10
	for i := 0; i < n; i++ {
		m.sessions = append(m.sessions, Session{
			ID: fmt.Sprintf("ses_%d", i), Title: fmt.Sprintf("session %d", i),
			Directory: "/mnt/c/project", FirstUserMsg: "question",
			TimeUpdated: int64(n - i),
		})
	}
	m.filtered = m.sessions
	m.cursor = cursor
	return m
}

func updateModel(m model, msg tea.Msg) (model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(model), cmd
}

func press(m model, key tea.KeyType) (model, tea.Cmd) {
	return updateModel(m, tea.KeyMsg{Type: key})
}

func assertMarked(t *testing.T, m model, want ...string) {
	t.Helper()
	got := sessionIDs(m.actionTargets())
	// No marks means a single focused row may still be an action target.
	if len(want) == 0 {
		if len(m.markedIDs) != 0 || m.selectionAnchor != "" {
			t.Fatalf("selection not cleared: %v, anchor %q", m.markedIDs, m.selectionAnchor)
		}
		return
	}
	if !reflect.DeepEqual(got, want) || len(m.markedIDs) != len(want) {
		t.Fatalf("marked targets = %v (%v), want %v", got, m.markedIDs, want)
	}
}

func TestShiftSelectionExpandsShrinksAndCrossesAnchor(t *testing.T) {
	m := selectionModel(6, 2)
	steps := []struct {
		key    tea.KeyType
		cursor int
		ids    []string
	}{
		{tea.KeyShiftDown, 3, []string{"ses_2", "ses_3"}},
		{tea.KeyShiftDown, 4, []string{"ses_2", "ses_3", "ses_4"}},
		{tea.KeyShiftUp, 3, []string{"ses_2", "ses_3"}},
		{tea.KeyShiftUp, 2, []string{"ses_2"}},
		{tea.KeyShiftUp, 1, []string{"ses_1", "ses_2"}},
		{tea.KeyShiftDown, 2, []string{"ses_2"}},
	}
	for _, step := range steps {
		m, _ = press(m, step.key)
		assertMarked(t, m, step.ids...)
		if m.cursor != step.cursor || m.selectionAnchor != "ses_2" {
			t.Fatalf("cursor %d, anchor %q", m.cursor, m.selectionAnchor)
		}
	}
}

func TestShiftSelectionBoundariesAndScrolling(t *testing.T) {
	m := selectionModel(12, 0)
	m, _ = press(m, tea.KeyShiftUp)
	assertMarked(t, m, "ses_0")
	for i := 0; i < 20; i++ {
		m, _ = press(m, tea.KeyShiftDown)
	}
	if m.cursor != 11 || len(m.markedIDs) != 12 || m.scrollOff != 7 {
		t.Fatalf("cursor=%d selected=%d scroll=%d", m.cursor, len(m.markedIDs), m.scrollOff)
	}
	for i := 0; i < 20; i++ {
		m, _ = press(m, tea.KeyShiftUp)
	}
	assertMarked(t, m, "ses_0")
	if m.scrollOff != 0 {
		t.Fatalf("scroll did not return to the top: %d", m.scrollOff)
	}
	for _, cursor := range []int{-1, 0} {
		empty := selectionModel(0, cursor)
		empty, _ = press(empty, tea.KeyShiftDown)
		assertMarked(t, empty)
		if len(empty.actionTargets()) != 0 {
			t.Fatal("empty results have action targets")
		}
	}
	m = selectionModel(3, -1)
	m, _ = press(m, tea.KeyShiftDown)
	assertMarked(t, m, "ses_0")
}

func TestOrdinaryNavigationRestoresSingleSelection(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd} {
		t.Run(key.String(), func(t *testing.T) {
			m := selectionModel(10, 3)
			m, _ = press(m, tea.KeyShiftDown)
			m, _ = press(m, key)
			assertMarked(t, m)
			if len(m.actionTargets()) > 1 {
				t.Fatal("ordinary navigation still has bulk targets")
			}
		})
	}
}

func TestOnlyShiftArrowsSelectAndEscapeStillQuits(t *testing.T) {
	m := selectionModel(3, 0)
	for _, key := range []tea.KeyType{tea.KeyCtrlA, tea.KeyNull} {
		m, _ = press(m, key)
		assertMarked(t, m)
	}
	m, _ = press(m, tea.KeyShiftDown)
	m, cmd := press(m, tea.KeyEsc)
	if cmd == nil {
		t.Fatal("Esc did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Esc no longer directly quits")
	}
	assertMarked(t, m, "ses_0", "ses_1")
}

func TestSearchChangesClearSelectionAndConfirmation(t *testing.T) {
	m := selectionModel(3, 0)
	m, _ = press(m, tea.KeyShiftDown)
	m, _ = press(m, tea.KeyCtrlD)
	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("question")})
	assertMarked(t, m)
	if m.pendingAction != "" || m.textInput.Value() != "question" {
		t.Fatalf("pending=%q query=%q", m.pendingAction, m.textInput.Value())
	}
	m, _ = press(m, tea.KeyShiftDown)
	m, _ = press(m, tea.KeyCtrlW)
	assertMarked(t, m)
	if m.textInput.Value() != "" {
		t.Fatal("Ctrl+W stopped deleting keywords")
	}
	m.cursor = 0
	m, _ = press(m, tea.KeyShiftDown)
	m, _ = press(m, tea.KeyLeft)
	if len(m.markedIDs) != 2 {
		t.Fatal("moving the text caret cleared marks without changing the query")
	}
}

func TestBackgroundLoadingPreservesIDsAndConfirmation(t *testing.T) {
	m := selectionModel(5, 1)
	m, _ = press(m, tea.KeyShiftDown)
	m, _ = press(m, tea.KeyCtrlD)
	before := append([]Session(nil), m.pendingTargets...)
	m, _ = updateModel(m, moreSessionsLoadedMsg{sessions: []Session{
		{ID: "new_first", Directory: "/mnt/c/new", TimeUpdated: 100},
	}})
	assertMarked(t, m, "ses_1", "ses_2")
	if m.focusedID() != "ses_2" || m.selectionAnchor != "ses_1" || m.cursor != 3 {
		t.Fatalf("background loading changed focus or anchor: %+v", m)
	}
	if !reflect.DeepEqual(before, m.pendingTargets) {
		t.Fatal("background results changed the set being confirmed")
	}
	m, cmd := press(m, tea.KeyCtrlD)
	if cmd == nil || m.batch == nil || !sameTargets(m.batch.targets, before) {
		t.Fatal("confirmation did not act on the original IDs")
	}
}

func TestHistoryLoadingDoesNotSelectNewMatches(t *testing.T) {
	m := selectionModel(4, 0)
	m.textInput.SetValue("match")
	m.sessions[0].Title = "match zero"
	m.sessions[2].Title = "match two"
	m.refilter("")
	m, _ = press(m, tea.KeyShiftDown)
	assertMarked(t, m, "ses_0", "ses_2")
	m, _ = updateModel(m, msgsLoadedMsg{msgs: map[string][]string{"ses_1": {"match in history"}}})
	assertMarked(t, m, "ses_0", "ses_2")
	if m.focusedID() != "ses_2" || len(m.filtered) != 3 {
		t.Fatal("history arrival moved focus or failed to add a result")
	}
}

func TestBatchConfirmationsAndSnapshots(t *testing.T) {
	m := selectionModel(9, 1)
	m, _ = press(m, tea.KeyShiftDown)
	m, cmd := press(m, tea.KeyCtrlD)
	if cmd != nil || m.pendingAction != "delete" || len(m.pendingTargets) != 2 || m.batch != nil {
		t.Fatal("first Ctrl+D must only ask for confirmation")
	}
	if !strings.Contains(m.renderStatusBar(), "child sessions") {
		t.Fatal("deletion confirmation must warn about child sessions")
	}
	m, _ = press(m, tea.KeyShiftDown)
	if m.pendingAction != "" {
		t.Fatal("selection change did not cancel confirmation")
	}
	m, cmd = press(m, tea.KeyCtrlD)
	if cmd != nil || len(m.pendingTargets) != 3 {
		t.Fatal("new selection was not reconfirmed")
	}
	m, cmd = press(m, tea.KeyCtrlD)
	if cmd == nil || m.batch == nil || len(m.batch.targets) != 3 || m.pendingAction != "" {
		t.Fatal("second Ctrl+D did not start the batch")
	}
	if !strings.Contains(m.renderStatusBar(), "Deleting 1/3") {
		t.Fatal("missing initial batch progress")
	}
	if !reflect.DeepEqual(sessionIDs(m.batch.targets), []string{"ses_1", "ses_2", "ses_3"}) {
		t.Fatal("wrong batch targets")
	}

	// Small opens start immediately. More than five tabs needs a second Enter.
	for _, count := range []int{1, 2, 5, 6} {
		m = selectionModel(count, 0)
		for i := 1; i < count; i++ {
			m, _ = press(m, tea.KeyShiftDown)
		}
		m, cmd = press(m, tea.KeyEnter)
		if count <= bulkOpenConfirmLimit {
			if cmd == nil || m.batch == nil || len(m.batch.targets) != count {
				t.Fatalf("open %d should start immediately", count)
			}
		} else {
			if cmd != nil || m.pendingAction != "open" || m.batch != nil {
				t.Fatal("large open did not ask for confirmation")
			}
			m, cmd = press(m, tea.KeyEnter)
			if cmd == nil || m.batch == nil || len(m.batch.targets) != count {
				t.Fatal("second Enter did not start the confirmed open")
			}
		}
	}
}

func TestBatchProgressFailuresAndRetry(t *testing.T) {
	for _, action := range []string{"delete", "open"} {
		t.Run(action, func(t *testing.T) {
			m := selectionModel(5, 0)
			m, _ = press(m, tea.KeyShiftDown)
			m, _ = press(m, tea.KeyShiftDown)
			_ = m.requestAction(action)
			if m.batch == nil {
				_ = m.requestAction(action)
			}
			// Only synthetic worker messages below; no real session is mutated.
			m, cmd := updateModel(m, batchItemDoneMsg{action: action, id: "ses_0"})
			if cmd == nil || m.batch.done != 1 || m.batch.succeeded != 1 {
				t.Fatal("did not advance one item at a time")
			}
			if (sessionIndex(m.sessions, "ses_0") < 0) != (action == "delete") {
				t.Fatal("incorrect session removal")
			}
			m, _ = updateModel(m, batchItemDoneMsg{action: action, id: "ses_1", err: errors.New("test failure")})
			if !strings.Contains(m.batchStatus(), "1 failed") {
				t.Fatal("missing failure progress")
			}
			m, cmd = updateModel(m, batchItemDoneMsg{action: action, id: "ses_2"})
			if cmd != nil || m.batch != nil || m.noticeOK || !strings.Contains(m.notice, "2 succeeded, 1 failed") {
				t.Fatalf("incorrect batch completion: %q", m.notice)
			}
			assertMarked(t, m, "ses_1")
			if sessionIndex(m.sessions, "ses_1") < 0 {
				t.Fatal("failed delete removed its session")
			}
			if m.actionTargets()[0].ID != "ses_1" {
				t.Fatal("retry would act on the cursor rather than the failure")
			}
			_ = m.requestAction(action)
			if m.batch == nil {
				_ = m.requestAction(action)
			}
			if len(m.batch.targets) != 1 || m.batch.targets[0].ID != "ses_1" {
				t.Fatal("retry set is incorrect")
			}
			m, _ = updateModel(m, batchItemDoneMsg{action: action, id: "ses_1"})
			assertMarked(t, m)
			if !m.noticeOK {
				t.Fatal("successful retry reported failure")
			}
		})
	}
}

func TestBatchIgnoresDuplicateAndUnrelatedResultsAndKeys(t *testing.T) {
	m := selectionModel(3, 0)
	m, _ = press(m, tea.KeyShiftDown)
	_ = m.requestAction("open")
	m, cmd := updateModel(m, batchItemDoneMsg{action: "delete", id: "ses_0"})
	if cmd != nil || m.batch.done != 0 {
		t.Fatal("unrelated result changed batch progress")
	}
	m, _ = updateModel(m, batchItemDoneMsg{action: "open", id: "ses_0"})
	m, cmd = updateModel(m, batchItemDoneMsg{action: "open", id: "ses_0"})
	if cmd != nil || m.batch.done != 1 {
		t.Fatal("duplicate result advanced the batch")
	}
	for _, key := range []tea.KeyType{tea.KeyEnter, tea.KeyCtrlD, tea.KeyShiftDown, tea.KeyDown} {
		m, cmd = press(m, key)
		if cmd != nil || m.batch.done != 1 || m.cursor != 1 {
			t.Fatal("busy batch accepted another action or selection change")
		}
	}
	m, _ = updateModel(m, batchItemDoneMsg{action: "open", id: "ses_1"})
	m, cmd = updateModel(m, batchItemDoneMsg{action: "open", id: "ses_1"})
	if cmd != nil || m.batch != nil {
		t.Fatal("late result restarted a finished batch")
	}
}

func TestBatchItemCmdUsesInjectedWorker(t *testing.T) {
	s := Session{ID: "test"}
	calls := 0
	errTest := errors.New("expected error")
	cmd := batchItemCmd("delete", s, func(got Session) error {
		calls++
		if got.ID != s.ID {
			t.Fatal("wrong worker target")
		}
		return errTest
	})
	if calls != 0 {
		t.Fatal("worker was called synchronously")
	}
	msg := cmd().(batchItemDoneMsg)
	if calls != 1 || msg.id != s.ID || msg.action != "delete" || msg.err != errTest {
		t.Fatal("wrong worker result")
	}
}

func TestChangedTargetsRequireNewConfirmation(t *testing.T) {
	m := selectionModel(3, 0)
	m, _ = press(m, tea.KeyCtrlD)
	// Simulate a result disappearing between the two confirmation presses.
	m.sessions = m.sessions[1:]
	m.refilter("ses_0")
	m, cmd := press(m, tea.KeyCtrlD)
	if cmd != nil || m.batch != nil || m.pendingTargets[0].ID != "ses_1" {
		t.Fatal("changed targets were executed without fresh confirmation")
	}
}

func TestFailedTargetsNeverFallBackToCursor(t *testing.T) {
	m := selectionModel(2, 0)
	m.markedIDs = map[string]bool{"no_longer_visible": true}
	if len(m.actionTargets()) != 0 {
		t.Fatal("invisible marks must not target an unrelated cursor row")
	}
	if cmd := m.requestAction("delete"); cmd != nil || m.pendingAction != "" {
		t.Fatal("started a delete on an unrelated row")
	}
}

func TestSuccessfulDeleteCannotBeResurrectedByStartup(t *testing.T) {
	m := selectionModel(3, 0)
	deleted := m.sessions[0]
	m.removeSession(deleted.ID)
	m, _ = updateModel(m, moreSessionsLoadedMsg{sessions: []Session{deleted}})
	m, _ = updateModel(m, msgsLoadedMsg{msgs: map[string][]string{deleted.ID: {"late history"}}})
	if sessionIndex(m.sessions, deleted.ID) >= 0 || len(m.allMsgs[deleted.ID]) != 0 {
		t.Fatal("late startup data resurrected a deleted session")
	}
}

func TestProjectPathsDeduplicateAndPreserveOrder(t *testing.T) {
	paths := projectPaths([]Session{
		{Directory: "/mnt/c/项目 with spaces"},
		{Directory: "/mnt/c/项目 with spaces"},
		{Directory: "/mnt/d/another"},
		{Directory: "/mnt/d/ANOTHER"},
		{Directory: "/root/CaseSensitive"},
		{Directory: "/root/casesensitive"},
	})
	want := []string{`C:\项目 with spaces`, `D:\another`, "/root/CaseSensitive", "/root/casesensitive"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths=%v want=%v", paths, want)
	}
}

func TestSelectionRenderingAndQuestionPickerStaySeparate(t *testing.T) {
	m := selectionModel(4, 1)
	m, _ = press(m, tea.KeyShiftDown)
	if !strings.Contains(m.renderStatusBar(), "2 selected") {
		t.Fatal("status bar does not display selection count")
	}
	for _, s := range m.filtered {
		row := m.renderItemRow(s, s.ID == m.focusedID(), nil)
		if strings.Contains(row, "[✓]") != m.markedIDs[s.ID] {
			t.Fatal("selection marker on wrong row")
		}
		if w := lipgloss.Width(row); w != colTotal {
			t.Fatalf("marker changed column alignment: width=%d want=%d", w, colTotal)
		}
	}
	m, _ = press(m, tea.KeyCtrlD)
	m, cmd := updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}, Alt: true})
	if cmd == nil || m.questionSess.ID != "ses_2" || m.questionMode != 1 || m.pendingAction != "" {
		t.Fatal("Alt+S must still operate on the focused session only")
	}
	before := sessionIDs(m.actionTargets())
	m, _ = press(m, tea.KeyShiftDown)
	if !reflect.DeepEqual(before, sessionIDs(m.actionTargets())) {
		t.Fatal("question picker Shift+arrow changed main list selection")
	}
}
