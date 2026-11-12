package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Session is a top-level OpenCode v2 session.
type Session struct {
	ID           string
	Title        string
	Slug         string
	Directory    string
	ModelID      string
	Agent        string
	TimeUpdated  int64
	FirstUserMsg string
}

// UserQuestion is one user message in a session, used by the Alt+S picker.
type UserQuestion struct {
	ID          string
	TimeCreated int64
	Text        string
}

// dbPath returns the OpenCode v2 database path. OPENCODE_DB is honored the
// same way OpenCode honors it; relative values resolve under the data dir.
func dbPath() string {
	home, _ := os.UserHomeDir()
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	if p := strings.TrimSpace(os.Getenv("OPENCODE_DB")); p != "" {
		if p == ":memory:" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(dataHome, "opencode", p)
	}
	return filepath.Join(dataHome, "opencode", "opencode.db")
}

func openDB(dbFile string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// One connection keeps the read snapshot consistent and avoids holding
	// several SQLite handles open on a WAL database.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA query_only = ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set query_only: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}
	return db, nil
}

// LoadSessions loads the most recent `limit` top-level sessions (limit <= 0
// means all), together with each session's first user message.
func LoadSessions(dbFile string, limit int) ([]Session, error) {
	return loadSessions(dbFile, limit, nil)
}

// LoadSessionsExcluding loads every top-level session except excludeIDs.
// Used to fetch the remainder after the initial recent-session batch.
func LoadSessionsExcluding(dbFile string, excludeIDs []string) ([]Session, error) {
	return loadSessions(dbFile, 0, excludeIDs)
}

func loadSessions(dbFile string, limit int, excludeIDs []string) ([]Session, error) {
	db, err := openDB(dbFile)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var sessions []Session
	if err := loadSessionRows(db, &sessions, limit, excludeIDs); err != nil {
		return nil, err
	}
	if err := loadFirstUserTexts(db, sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func loadSessionRows(db *sql.DB, sessions *[]Session, limit int, excludeIDs []string) error {
	query := `
		SELECT s.id, COALESCE(s.title, ''), s.slug, s.directory,
		       s.model, COALESCE(s.agent, ''), s.time_updated
		FROM session_v2 s
		WHERE s.time_archived IS NULL
		  AND (s.parent_id IS NULL OR s.parent_id = '')`
	var args []interface{}
	if len(excludeIDs) > 0 {
		placeholders := make([]string, len(excludeIDs))
		for i, id := range excludeIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND s.id NOT IN (" + strings.Join(placeholders, ",") + ")"
	}
	query += " ORDER BY s.time_updated DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s Session
		var modelJSON sql.NullString
		if err := rows.Scan(&s.ID, &s.Title, &s.Slug, &s.Directory, &modelJSON, &s.Agent, &s.TimeUpdated); err != nil {
			return fmt.Errorf("scan session: %w", err)
		}
		s.ModelID = parseModelID(modelJSON.String)
		if s.Title == "" {
			s.Title = s.Slug
		}
		*sessions = append(*sessions, s)
	}
	return rows.Err()
}

func parseModelID(raw string) string {
	if raw == "" {
		return ""
	}
	var m struct {
		ID         string `json:"id"`
		ProviderID string `json:"providerID"`
		Variant    string `json:"variant"`
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	model := m.ID
	if m.ProviderID != "" && !strings.EqualFold(m.ProviderID, m.ID) {
		model = m.ProviderID + "/" + model
	}
	if m.Variant != "" {
		model += "#" + m.Variant
	}
	return model
}

func loadFirstUserTexts(db *sql.DB, sessions []Session) error {
	if len(sessions) == 0 {
		return nil
	}
	placeholders := make([]string, len(sessions))
	args := make([]interface{}, len(sessions))
	for i, s := range sessions {
		placeholders[i] = "?"
		args[i] = s.ID
	}

	rows, err := db.Query(`
		SELECT session_id, data FROM session_message
		WHERE type = 'user'
		  AND session_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY session_id, seq
	`, args...)
	if err != nil {
		return fmt.Errorf("query user msgs: %w", err)
	}
	defer rows.Close()

	firstMsgPerSession := make(map[string]string)
	for rows.Next() {
		var sid, data string
		if err := rows.Scan(&sid, &data); err != nil {
			continue
		}
		if _, ok := firstMsgPerSession[sid]; ok {
			continue
		}
		if text := extractUserText(data); text != "" {
			firstMsgPerSession[sid] = text
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range sessions {
		sessions[i].FirstUserMsg = firstMsgPerSession[sessions[i].ID]
	}
	return nil
}

func extractUserText(raw string) string {
	var msg struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return ""
	}
	return msg.Text
}

// assistantContentItem decodes only text content. Reasoning and tool items are
// scanned but their large payloads are deliberately not retained.
type assistantContentItem struct {
	Type string
	Text string
}

func (c *assistantContentItem) UnmarshalJSON(data []byte) error {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil
	}
	c.Type = head.Type
	if head.Type != "text" {
		return nil
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil
	}
	c.Text = body.Text
	return nil
}

func extractAssistantTexts(raw string) []string {
	var msg struct {
		Content []assistantContentItem `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return nil
	}
	out := make([]string, 0, 1)
	for _, item := range msg.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			out = append(out, item.Text)
		}
	}
	return out
}

// LoadAllMessages loads every user prompt and assistant text response, keyed
// by session ID. Tool output, reasoning, system instructions and synthetic
// reminders are intentionally excluded.
func LoadAllMessages(dbFile string) (map[string][]string, error) {
	db, err := openDB(dbFile)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT session_id, type, data FROM session_message
		WHERE type = 'user'
		   OR (type = 'assistant'
		       AND (instr(data, '"type":"text"') > 0
		            OR instr(data, '"type": "text"') > 0))
		ORDER BY session_id, seq
	`)
	if err != nil {
		return nil, fmt.Errorf("query texts: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var sid, msgType, data string
		if err := rows.Scan(&sid, &msgType, &data); err != nil {
			continue
		}
		switch msgType {
		case "user":
			if text := extractUserText(data); strings.TrimSpace(text) != "" {
				result[sid] = append(result[sid], text)
			}
		case "assistant":
			result[sid] = append(result[sid], extractAssistantTexts(data)...)
		}
	}
	return result, rows.Err()
}

// LoadUserQuestions returns all user messages for a session, newest first.
func LoadUserQuestions(dbFile, sessionID string) ([]UserQuestion, error) {
	db, err := openDB(dbFile)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, time_created, data FROM session_message
		WHERE session_id = ? AND type = 'user'
		ORDER BY seq DESC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query user questions: %w", err)
	}
	defer rows.Close()

	var questions []UserQuestion
	for rows.Next() {
		var q UserQuestion
		var data string
		if err := rows.Scan(&q.ID, &q.TimeCreated, &data); err != nil {
			continue
		}
		q.Text = extractUserText(data)
		if strings.TrimSpace(q.Text) == "" {
			q.Text = "(no text; may contain attachments)"
		}
		questions = append(questions, q)
	}
	return questions, rows.Err()
}

// NextMessageID returns the first message after messageID in the session, or
// an empty string when messageID is the last message.
func NextMessageID(dbFile, sessionID, messageID string) (string, error) {
	db, err := openDB(dbFile)
	if err != nil {
		return "", err
	}
	defer db.Close()

	var next string
	err = db.QueryRow(`
		SELECT id FROM session_message
		WHERE session_id = ?
		  AND seq > (SELECT seq FROM session_message WHERE id = ? AND session_id = ?)
		ORDER BY seq ASC LIMIT 1
	`, sessionID, messageID, sessionID).Scan(&next)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query next message: %w", err)
	}
	return next, nil
}

// MonitorSession is the info + message flow for a single session monitor.
type MonitorSession struct {
	ID           string
	Title        string
	Agent        string
	ModelID      string
	TokensIn     int64
	TokensOut    int64
	Cost         float64
	SessionCount int
	Messages     []MonitorMessage
}

type MonitorMessage struct {
	TimeCreated int64
	Role        string
	Agent       string
	Title       string
	ModelID     string
	Text        string
}

func LoadMonitor(dbFile, sessionID string) (*MonitorSession, error) {
	db, err := openDB(dbFile)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	ms := &MonitorSession{ID: sessionID}

	if sessionID == "" {
		err = db.QueryRow(`
			SELECT COUNT(*) FROM session_v2
			WHERE parent_id IS NULL OR parent_id = ''
		`).Scan(&ms.SessionCount)
		if err != nil {
			return nil, fmt.Errorf("query sessions: %w", err)
		}
	} else {
		var modelJSON sql.NullString
		err = db.QueryRow(`
			SELECT COALESCE(title, ''), COALESCE(agent, ''), model,
			       tokens_input, tokens_output, cost
			FROM session_v2 WHERE id = ?
		`, sessionID).Scan(&ms.Title, &ms.Agent, &modelJSON, &ms.TokensIn, &ms.TokensOut, &ms.Cost)
		if err != nil {
			return nil, fmt.Errorf("query session: %w", err)
		}
		ms.ModelID = parseModelID(modelJSON.String)
	}

	type msgRow struct {
		id        string
		sessionID string
		time      int64
		msgType   string
		role      string
		agent     string
		model     string
		text      string
	}
	var msgRows []msgRow

	appendRow := func(sessionID, msgType, data string, created int64, id string) {
		switch msgType {
		case "user":
			text := extractUserText(data)
			if strings.TrimSpace(text) == "" {
				return
			}
			msgRows = append(msgRows, msgRow{
				id: id, sessionID: sessionID, time: created,
				msgType: msgType, role: "user", text: text,
			})
		case "assistant":
			texts := extractAssistantTexts(data)
			if len(texts) == 0 {
				return
			}
			agent, model := parseAssistantMeta(data)
			msgRows = append(msgRows, msgRow{
				id: id, sessionID: sessionID, time: created,
				msgType: msgType, role: "assistant", agent: agent,
				model: model, text: texts[0],
			})
		}
	}

	var rows *sql.Rows
	if sessionID == "" {
		rows, err = db.Query(`
			SELECT m.id, m.session_id, m.time_created, m.type, m.data
			FROM session_message m
			WHERE m.session_id IN (
				SELECT id FROM session_v2 WHERE parent_id IS NULL OR parent_id = ''
			)
			ORDER BY m.time_created DESC LIMIT 300
		`)
	} else {
		rows, err = db.Query(`
			WITH RECURSIVE subs(id) AS (
				SELECT id FROM session_v2 WHERE id = ?
				UNION ALL
				SELECT s.id FROM session_v2 s JOIN subs ON s.parent_id = subs.id
			)
			SELECT m.id, m.session_id, m.time_created, m.type, m.data
			FROM session_message m
			WHERE m.session_id IN (SELECT id FROM subs)
			ORDER BY m.time_created ASC
		`, sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	for rows.Next() {
		var id, sid, msgType, data string
		var created int64
		if err := rows.Scan(&id, &sid, &created, &msgType, &data); err != nil {
			continue
		}
		appendRow(sid, msgType, data, created, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if sessionID == "" {
		for i, j := 0, len(msgRows)-1; i < j; i, j = i+1, j-1 {
			msgRows[i], msgRows[j] = msgRows[j], msgRows[i]
		}
	}

	titleByID := make(map[string]string)
	if len(msgRows) > 0 {
		placeholders := make([]string, 0, len(msgRows))
		args := make([]interface{}, 0, len(msgRows))
		seen := make(map[string]bool)
		for _, r := range msgRows {
			if seen[r.sessionID] {
				continue
			}
			seen[r.sessionID] = true
			placeholders = append(placeholders, "?")
			args = append(args, r.sessionID)
		}
		trows, err := db.Query(`
			SELECT id, COALESCE(title, '') FROM session_v2
			WHERE id IN (`+strings.Join(placeholders, ",")+`)
		`, args...)
		if err != nil {
			return nil, fmt.Errorf("query session titles: %w", err)
		}
		for trows.Next() {
			var id, title string
			if err := trows.Scan(&id, &title); err != nil {
				continue
			}
			titleByID[id] = title
		}
		trows.Close()
		if err := trows.Err(); err != nil {
			return nil, err
		}
	}

	for _, r := range msgRows {
		ms.Messages = append(ms.Messages, MonitorMessage{
			TimeCreated: r.time,
			Role:        r.role,
			Agent:       r.agent,
			Title:       titleByID[r.sessionID],
			ModelID:     r.model,
			Text:        r.text,
		})
	}
	return ms, nil
}

func parseAssistantMeta(raw string) (agent, model string) {
	var msg struct {
		Agent string `json:"agent"`
		Model struct {
			ID         string `json:"id"`
			ProviderID string `json:"providerID"`
			Variant    string `json:"variant"`
		} `json:"model"`
	}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return "", ""
	}
	agent = msg.Agent
	model = msg.Model.ID
	if msg.Model.ProviderID != "" && !strings.EqualFold(msg.Model.ProviderID, msg.Model.ID) {
		model = msg.Model.ProviderID + "/" + model
	}
	if msg.Model.Variant != "" {
		model += "#" + msg.Model.Variant
	}
	return agent, model
}
