//go:build !linux

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func opencodeBinary() (string, error) {
	return exec.LookPath("opencode")
}

func openSessionBg(s Session) error {
	bin, err := opencodeBinary()
	if err != nil {
		return fmt.Errorf("opencode not found: %w", err)
	}
	cmd := exec.Command(bin, "-s", s.ID)
	cmd.Dir = s.Directory
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

func spawnMonitor(sessionID string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get self path: %w", err)
	}
	args := []string{"-m"}
	if sessionID != "" {
		args = append(args, sessionID)
	}
	cmd := exec.Command(self, args...)
	return cmd.Start()
}
