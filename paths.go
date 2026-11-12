package main

import "strings"

// toWindowsPath converts a WSL mount path such as /mnt/c/ws/project to
// C:\ws\project. Paths outside /mnt/<drive> are returned unchanged.
func toWindowsPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/mnt/") {
		return p
	}
	rest := p[len("/mnt/"):]
	if rest == "" {
		return p
	}
	drive := rest[0]
	if drive < 'a' || drive > 'z' {
		if drive < 'A' || drive > 'Z' {
			return p
		}
	}
	if len(rest) == 1 {
		return strings.ToUpper(string(drive)) + ":"
	}
	if rest[1] != '/' {
		return p
	}
	return strings.ToUpper(string(drive)) + ":" + strings.ReplaceAll(rest[1:], "/", "\\")
}

// windowsToWSL converts C:\ws\project to /mnt/c/ws/project. Inputs that are
// not drive-letter paths are returned unchanged.
func windowsToWSL(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if len(p) < 2 || p[1] != ':' {
		return p
	}
	drive := strings.ToLower(string(p[0]))
	if drive < "a" || drive > "z" {
		return p
	}
	return "/mnt/" + drive + p[2:]
}

func isWindowsPath(p string) bool {
	return len(p) >= 2 && p[1] == ':'
}
