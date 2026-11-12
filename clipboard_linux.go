//go:build linux

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os/exec"
	"unicode/utf16"
)

// clipboardWrite copies text to the Windows clipboard through WSL interop.
// clip.exe auto-detects UTF-16LE from the NUL bytes; do not prepend a BOM or
// the pasted text starts with a zero-width character.
func clipboardWrite(text string) error {
	clip := windowsExe("clip.exe")
	if clip == "" {
		return fmt.Errorf("clip.exe not found")
	}
	cmd := exec.Command(clip)
	cmd.Dir = "/mnt/c/Windows"
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var buf bytes.Buffer
	for _, unit := range utf16.Encode([]rune(text)) {
		_ = binary.Write(&buf, binary.LittleEndian, unit)
	}
	if _, err := stdin.Write(buf.Bytes()); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return err
	}
	if err := stdin.Close(); err != nil {
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}
