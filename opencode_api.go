package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// serviceState is ~/.local/state/opencode/service.json. OpenCode v2 always
// authenticates with the username "opencode"; only the password is random.
type serviceState struct {
	URL      string `json:"url"`
	Password string `json:"password"`
	PID      int    `json:"pid"`
}

func serviceStatePath() string {
	home, _ := os.UserHomeDir()
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "opencode", "service.json")
}

func readServiceState() (*serviceState, error) {
	raw, err := os.ReadFile(serviceStatePath())
	if err != nil {
		return nil, err
	}
	var st serviceState
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("decode service.json: %w", err)
	}
	if st.URL == "" || st.Password == "" {
		return nil, fmt.Errorf("service.json is missing url or password")
	}
	st.URL = strings.TrimRight(st.URL, "/")
	return &st, nil
}

// serviceHTTPClient deliberately bypasses HTTP_PROXY: the OpenCode service is
// always loopback, and a global proxy would break API calls when NO_PROXY does
// not include 127.0.0.1.
var serviceHTTPClient = &http.Client{
	Timeout:   8 * time.Second,
	Transport: &http.Transport{Proxy: nil},
}

func (st *serviceState) request(method, path string, body interface{}) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, st.URL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.SetBasicAuth("opencode", st.Password)

	client := serviceHTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := strings.TrimSpace(string(raw))
		if detail == "" {
			detail = resp.Status
		}
		return nil, fmt.Errorf("opencode api returned %s: %s", resp.Status, detail)
	}
	return raw, nil
}

func (st *serviceState) healthy() bool {
	_, err := st.request(http.MethodGet, "/api/info", nil)
	return err == nil
}

// ensureService returns a healthy background service, starting it if needed.
func ensureService() (*serviceState, error) {
	if st, err := readServiceState(); err == nil && st.healthy() {
		return st, nil
	}

	bin, err := opencodeBinary()
	if err != nil {
		return nil, fmt.Errorf("opencode not found: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "service", "start")
	cmd.Dir, _ = os.UserHomeDir()
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	_ = cmd.Run()

	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		st, err := readServiceState()
		if err == nil && st.healthy() {
			return st, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("service is not responding")
		}
		time.Sleep(250 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timed out")
	}
	return nil, fmt.Errorf("opencode background service is unavailable: %w", lastErr)
}

// DeleteSession deletes a session (and its child sessions) through the v2 API,
// falling back to the CLI if the service cannot be reached.
func DeleteSession(id, directory string) error {
	if st, err := ensureService(); err == nil {
		if _, err := st.request(http.MethodDelete, "/api/session/"+url.PathEscape(id), nil); err == nil {
			return nil
		}
	}

	bin, err := opencodeBinary()
	if err != nil {
		return fmt.Errorf("opencode not found: %w", err)
	}
	cmd := exec.Command(bin, "session", "delete", id)
	if directory != "" {
		if st, statErr := os.Stat(directory); statErr == nil && st.IsDir() {
			cmd.Dir = directory
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return fmt.Errorf("%w: %s", err, detail)
		}
		return err
	}
	return nil
}

// ForkSession creates a fork before beforeID. An empty beforeID forks the
// whole session, which is exactly what is needed when the selected question is
// the last message.
func ForkSession(directory, sessionID, beforeID string) (string, error) {
	st, err := ensureService()
	if err != nil {
		return "", err
	}
	body := map[string]interface{}{"before": nil}
	if beforeID != "" {
		body["before"] = beforeID
	}
	raw, err := st.request(http.MethodPost, "/api/session/"+url.PathEscape(sessionID)+"/fork", body)
	if err != nil {
		return "", err
	}
	var result struct {
		ID   string `json:"id"`
		Data *struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode fork response: %w", err)
	}
	if result.ID != "" {
		return result.ID, nil
	}
	if result.Data != nil && result.Data.ID != "" {
		return result.Data.ID, nil
	}
	return "", fmt.Errorf("fork response did not contain a session id")
}

// ForkQuestion forks the session at the selected user question. The question
// itself is included in the fork (the next message is used as the boundary),
// then the fork is opened in a new tab and the question is copied so it can be
// pasted if it needs editing.
func ForkQuestion(dbFile string, s Session, q UserQuestion) (forkedID string, copied bool, err error) {
	beforeID, err := NextMessageID(dbFile, s.ID, q.ID)
	if err != nil {
		return "", false, err
	}
	forkedID, err = ForkSession(s.Directory, s.ID, beforeID)
	if err != nil {
		return "", false, err
	}
	if err := clipboardWrite(q.Text); err == nil {
		copied = true
	}
	if err := openSessionBg(Session{ID: forkedID, Directory: s.Directory}); err != nil {
		return forkedID, copied, fmt.Errorf("fork created (%s), but could not open it: %w", forkedID, err)
	}
	return forkedID, copied, nil
}
