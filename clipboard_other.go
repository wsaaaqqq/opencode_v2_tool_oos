//go:build !linux

package main

import "fmt"

func clipboardWrite(text string) error {
	return fmt.Errorf("clipboard is only implemented for the WSL build")
}
