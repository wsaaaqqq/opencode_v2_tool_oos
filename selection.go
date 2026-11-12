package main

import "strings"

// Keep identities, not result offsets: background loading can insert or reorder
// results while the user is selecting. Only an explicit Shift+arrow changes
// the marked range; an arriving result never becomes selected by itself.
func (m *model) clearSelection() {
	m.markedIDs = nil
	m.selectionAnchor = ""
}

func (m model) focusedID() string {
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		return m.filtered[m.cursor].ID
	}
	return ""
}

func sessionIndex(sessions []Session, id string) int {
	for i, s := range sessions {
		if s.ID == id {
			return i
		}
	}
	return -1
}

func (m *model) extendSelection(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		m.cursor = 0
		m.selectionAnchor = m.focusedID()
	} else {
		if sessionIndex(m.filtered, m.selectionAnchor) < 0 {
			m.selectionAnchor = m.focusedID()
		}
		m.cursor = max(0, min(len(m.filtered)-1, m.cursor+delta))
	}
	anchor := sessionIndex(m.filtered, m.selectionAnchor)
	lo, hi := min(anchor, m.cursor), max(anchor, m.cursor)
	m.markedIDs = make(map[string]bool, hi-lo+1)
	for _, s := range m.filtered[lo : hi+1] {
		m.markedIDs[s.ID] = true
	}
	m.scrollOff = calcScrollOff(m.scrollOff, m.cursor, m.visibleSlots())
}

// Multi-selection takes precedence over the cursor. Never silently fall back
// to an unrelated focused row when there are marked IDs.
func (m model) actionTargets() []Session {
	if len(m.markedIDs) > 0 {
		var targets []Session
		for _, s := range m.filtered {
			if m.markedIDs[s.ID] {
				targets = append(targets, s)
			}
		}
		return targets
	}
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		return []Session{m.filtered[m.cursor]}
	}
	return nil
}

// Refresh without changing the focused identity or adding marked IDs. The
// caller captures focus before merging background data or removing a session.
func (m *model) refilter(focusID string) {
	m.filtered = FilterSessions(m.sessions, ParseKeys(m.textInput.Value()), m.msgMap())
	if i := sessionIndex(m.filtered, focusID); i >= 0 {
		m.cursor = i
	} else {
		m.cursor = clampCursor(m.cursor, len(m.filtered))
	}
	visible := make(map[string]bool, len(m.filtered))
	for _, s := range m.filtered {
		visible[s.ID] = true
	}
	for id := range m.markedIDs {
		if !visible[id] {
			delete(m.markedIDs, id)
		}
	}
	if !visible[m.selectionAnchor] {
		m.selectionAnchor = ""
	}
	m.scrollOff = calcScrollOff(m.scrollOff, m.cursor, m.visibleSlots())
}

// Remember successful deletes so late startup messages cannot resurrect them.
func (m *model) removeSession(id string) {
	focusID := m.focusedID()
	if m.deletedIDs == nil {
		m.deletedIDs = make(map[string]bool)
	}
	m.deletedIDs[id] = true
	for i, s := range m.sessions {
		if s.ID == id {
			m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
			break
		}
	}
	delete(m.allMsgs, id)
	delete(m.markedIDs, id)
	m.refilter(focusID)
}

func (m model) withoutDeleted(sessions []Session) []Session {
	out := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if !m.deletedIDs[s.ID] {
			out = append(out, s)
		}
	}
	return out
}

func projectPaths(sessions []Session) []string {
	seen := make(map[string]bool)
	var paths []string
	for _, s := range sessions {
		p := toWindowsPath(s.Directory)
		key := p
		if isWindowsPath(p) {
			p = strings.ReplaceAll(p, "/", "\\")
			key = strings.ToLower(p)
		}
		if !seen[key] {
			seen[key] = true
			paths = append(paths, p)
		}
	}
	return paths
}
