//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func windowsExe(name string) string {
	for _, p := range []string{
		"/mnt/c/Windows/System32/" + name,
		"/mnt/c/Windows/" + name,
	} {
		if fileExists(p) {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func opencodeBinary() (string, error) {
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".opencode", "bin", "opencode")
	if fileExists(p) {
		return p, nil
	}
	return exec.LookPath("opencode")
}

func windowsTerminalPath() string {
	cmdExe := windowsExe("cmd.exe")
	if cmdExe == "" {
		return ""
	}
	cmd := exec.Command(cmdExe, "/d", "/c", "echo", "%LOCALAPPDATA%")
	cmd.Dir = "/mnt/c/Windows"
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	localAppData := strings.TrimSpace(strings.TrimRight(string(out), "\r\n"))
	if localAppData == "" {
		return ""
	}
	wt := filepath.Join(windowsToWSL(localAppData), "Microsoft", "WindowsApps", "wt.exe")
	if fileExists(wt) {
		return wt
	}
	return ""
}

func startWindows(exe string, dir string, args ...string) error {
	cmd := exec.Command(exe, args...)
	if dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			cmd.Dir = dir
		}
	}
	if cmd.Dir == "" {
		cmd.Dir = "/mnt/c/Windows"
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// openInNewTab opens a Windows Terminal tab (or a new console window when
// Windows Terminal is unavailable) running command.
func openInNewTab(command ...string) error {
	return openInNewTabAt("", command...)
}

func openInNewTabAt(windowsDir string, command ...string) error {
	cmdExe := windowsExe("cmd.exe")
	if cmdExe == "" {
		return fmt.Errorf("cmd.exe not found")
	}
	full := append([]string{"cmd.exe", "/k"}, command...)

	if wt := windowsTerminalPath(); wt != "" {
		args := []string{"-w", "0", "nt"}
		if windowsDir != "" {
			args = append(args, "-d", windowsDir)
		}
		args = append(args, full...)
		return startWindows(wt, "/mnt/c/Windows", args...)
	}

	args := append([]string{"/c", "start", "", "cmd.exe", "/k"}, command...)
	dir := "/mnt/c/Windows"
	if windowsDir != "" {
		if wslDir := windowsToWSL(windowsDir); wslDir != windowsDir {
			dir = wslDir
		}
	}
	return startWindows(cmdExe, dir, args...)
}

var (
	ooChecked bool
	ooOK      bool
)

// ooAvailable reports whether the user's `oo` command is on the Windows PATH.
// It is cached because PATH does not change while oos is running.
func ooAvailable() bool {
	if ooChecked {
		return ooOK
	}
	ooChecked = true
	cmdExe := windowsExe("cmd.exe")
	if cmdExe == "" {
		return false
	}
	cmd := exec.Command(cmdExe, "/d", "/c", "where", "oo")
	cmd.Dir = "/mnt/c/Windows"
	ooOK = cmd.Run() == nil
	return ooOK
}

func openSessionBg(s Session) error {
	dir := windowsToWSL(s.Directory)
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = home
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir, _ = os.UserHomeDir()
	}

	// The normal path mirrors the user's `oo` command: open the tab in the
	// project directory and let oo.cmd set UTF-8, the proxy and sing-box.
	if winDir := toWindowsPath(dir); isWindowsPath(winDir) && ooAvailable() {
		return openInNewTabAt(winDir, "oo", "-s", s.ID)
	}

	bin, err := opencodeBinary()
	if err != nil {
		return fmt.Errorf("opencode not found: %w", err)
	}
	return openInNewTab("wsl.exe", "--cd", dir, "-e", bin, "-s", s.ID)
}

func spawnMonitor(sessionID string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get self path: %w", err)
	}
	args := []string{"wsl.exe", "-e", self, "-m"}
	if sessionID != "" {
		args = append(args, sessionID)
	}
	return openInNewTab(args...)
}
