package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var version = "1.0.0"

func main() {
	args := os.Args[1:]
	dbFile := dbPath()
	printMode := false
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			printUsage()
			return
		case "-v", "--version":
			fmt.Println("oos", version)
			return
		case "--upgrade":
			fmt.Fprintln(os.Stderr, "This build has no auto-upgrade; rebuild from the source project.")
			return
		case "--db":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--db requires a path")
				os.Exit(1)
			}
			i++
			dbFile = args[i]
		case "--print":
			printMode = true
		case "-m":
			sessionID := ""
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				sessionID = args[i+1]
				i++
			}
			if err := checkDB(dbFile); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			if err := runMonitor(dbFile, sessionID); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			positional = append(positional, args[i])
		}
	}

	if err := checkDB(dbFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	query := strings.Join(positional, " ")
	if printMode {
		if err := runPrint(dbFile, query, 30); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if _, err := runTUI(dbFile, query); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func checkDB(dbFile string) error {
	if dbFile == ":memory:" {
		return nil
	}
	if _, err := os.Stat(dbFile); err != nil {
		return fmt.Errorf("database not found: %s", dbFile)
	}
	return nil
}

func printUsage() {
	fmt.Println(`oos — search OpenCode v2 sessions across all projects

Usage:
  oos [keywords...]          interactive search
  oos -m [sessionID]         live monitor (all sessions when ID omitted)
  oos --print [keywords...]  print matching sessions and exit
  oos --db <path>            use a specific database
  oos -v                     show version

Keywords are space-separated AND terms. Prefix a term with ! to exclude it.
On Windows run the oos.cmd wrapper; it starts this binary inside WSL.`)
}

func runPrint(dbFile, query string, limit int) error {
	sessions, err := LoadSessions(dbFile, 0)
	if err != nil {
		return err
	}
	msgs, err := LoadAllMessages(dbFile)
	if err != nil {
		return err
	}
	filter := ParseKeys(query)
	filtered := FilterSessions(sessions, filter, msgs)

	for i, s := range filtered {
		if limit > 0 && i >= limit {
			break
		}
		text := findBestMsg(s.FirstUserMsg, msgs[s.ID], filter.Include)
		text = oneLine(text)
		fmt.Printf("%s\t%s\t%s\t%s\n",
			time.UnixMilli(s.TimeUpdated).Format("2006-01-02 15:04"),
			toWindowsPath(s.Directory),
			s.Title,
			truncateCols(text, 120),
		)
	}
	fmt.Fprintf(os.Stderr, "%d matches\n", len(filtered))
	return nil
}
