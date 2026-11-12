package main

import (
	"os"
	"strings"
	"testing"
)

func testDB(t *testing.T) string {
	t.Helper()
	p := os.Getenv("OOS_TEST_DB")
	if p == "" {
		t.Skip("OOS_TEST_DB is not set")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("test database not found: %v", err)
	}
	return p
}

func TestLoadSessions(t *testing.T) {
	dbFile := testDB(t)
	sessions, err := LoadSessions(dbFile, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) == 0 {
		t.Fatal("expected at least one session")
	}
	for _, s := range sessions {
		if !strings.HasPrefix(s.ID, "ses_") {
			t.Fatalf("unexpected session id %q", s.ID)
		}
		if s.Directory == "" {
			t.Fatalf("session %s has no directory", s.ID)
		}
	}
}

func TestLoadAllMessages(t *testing.T) {
	dbFile := testDB(t)
	msgs, err := LoadAllMessages(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, texts := range msgs {
		total += len(texts)
	}
	if total == 0 {
		t.Fatal("expected some text messages")
	}
}

func TestLoadUserQuestionsAndMonitor(t *testing.T) {
	dbFile := testDB(t)
	sessions, err := LoadSessions(dbFile, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) == 0 {
		t.Skip("no sessions")
	}
	s := sessions[0]
	if _, err := LoadUserQuestions(dbFile, s.ID); err != nil {
		t.Fatal(err)
	}
	monitor, err := LoadMonitor(dbFile, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if monitor.ID != s.ID {
		t.Fatalf("monitor id = %q, want %q", monitor.ID, s.ID)
	}
	if len(monitor.Messages) == 0 {
		t.Fatal("expected monitor messages")
	}
	for _, m := range monitor.Messages {
		if m.Role == "" || m.Text == "" {
			t.Fatalf("incomplete monitor message: %+v", m)
		}
	}
	if _, err := LoadMonitor(dbFile, ""); err != nil {
		t.Fatal(err)
	}
}

func TestFilterSessions(t *testing.T) {
	dbFile := testDB(t)
	sessions, err := LoadSessions(dbFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := LoadAllMessages(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	filtered := FilterSessions(sessions, ParseKeys("opencode !java"), msgs)
	for _, s := range filtered {
		if strings.Contains(strings.ToLower(s.Directory), "java") {
			t.Fatalf("excluded directory matched: %s", s.Directory)
		}
	}
}

func TestForkSessionRoundTrip(t *testing.T) {
	if os.Getenv("OOS_TEST_FORK") == "" {
		t.Skip("OOS_TEST_FORK is not set")
	}
	dbFile := testDB(t)
	sessions, err := LoadSessions(dbFile, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) < 2 {
		t.Skip("need at least two sessions")
	}
	s := sessions[1]
	questions, err := LoadUserQuestions(dbFile, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) == 0 {
		t.Skip("session has no user questions")
	}
	beforeID, err := NextMessageID(dbFile, s.ID, questions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	forkedID, err := ForkSession(s.Directory, s.ID, beforeID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(forkedID, "ses_") {
		t.Fatalf("unexpected forked id %q", forkedID)
	}
	if err := DeleteSession(forkedID, s.Directory); err != nil {
		t.Fatalf("delete test fork: %v", err)
	}
}

func TestClipboard(t *testing.T) {
	if os.Getenv("OOS_TEST_CLIPBOARD") == "" {
		t.Skip("OOS_TEST_CLIPBOARD is not set")
	}
	if err := clipboardWrite("oos 剪贴板测试"); err != nil {
		t.Fatal(err)
	}
}

func TestPathConversion(t *testing.T) {
	cases := map[string]string{
		"/mnt/c/ws/project": "C:\\ws\\project",
		"/mnt/d":            "D:",
		"/root":             "/root",
		"/mnt/wsl/shared":   "/mnt/wsl/shared",
	}
	for in, want := range cases {
		if got := toWindowsPath(in); got != want {
			t.Errorf("toWindowsPath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := windowsToWSL(`C:\ws\project`); got != "/mnt/c/ws/project" {
		t.Errorf("windowsToWSL = %q", got)
	}
}
