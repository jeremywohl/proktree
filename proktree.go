package main

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"golang.org/x/term"
)


// CLI represents the command-line interface
type CLI struct {
	PIDs              []string `short:"p" name:"pid" help:"Show only parents and descendants of process PID (can be specified multiple times)"`
	Users             []string `short:"u" name:"user" help:"Show only parents and descendants of processes of USER (can be specified multiple times, defaults to current user if -u is used without argument)"`
	SearchStrings     []string `short:"s" name:"string" help:"Show only parents and descendants of process names containing STRING (can be specified multiple times)"`
	SearchStringsCase []string `short:"i" name:"string-insensitive" help:"Show only parents and descendants of process names containing STRING case-insensitively (can be specified multiple times)"`
	ShowFullUser      bool     `name:"long-users" help:"Show full usernames, without truncation"`
	ShowFullCommand   bool     `name:"long-commands" help:"Show full commands, without truncation"`
}

var cli CLI

func main() {
	// Parse command-line arguments
	args, userFlagWithoutArg := parseUserArgs(os.Args[1:])

	// Parse with modified args
	os.Args = append([]string{os.Args[0]}, args...)
	_ = kong.Parse(&cli,
		kong.Name("proktree"),
		kong.Description("Print your processes as a tree, nicely displayed"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: false,
		}),
	)

	// If -u was used without argument, add current user
	if userFlagWithoutArg {
		if currentUser, err := user.Current(); err == nil {
			cli.Users = append(cli.Users, currentUser.Username)
		}
	}

	// Get all processes using platform-specific implementation
	platform := GetPlatform()
	processes, err := platform.GetProcesses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	
	// Build process relationships
	pidToProcess := make(map[int]*Process)
	pidToChildren := make(map[int][]int)
	skipPids := make(map[int]bool)
	
	// Find proktree process to skip it and its children
	proktreePid := -1
	for i := range processes {
		p := &processes[i]
		pidToProcess[p.PID] = p
		if p.PPID > 0 {
			pidToChildren[p.PPID] = append(pidToChildren[p.PPID], p.PID)
		}
		
		// Mark proktree for skipping
		if strings.Contains(p.Command, "proktree") {
			proktreePid = p.PID
			skipPids[p.PID] = true
		}
	}
	
	// Skip ps children of proktree
	if proktreePid > 0 {
		for _, childPid := range pidToChildren[proktreePid] {
			if p, ok := pidToProcess[childPid]; ok && strings.HasPrefix(p.Command, "ps ") {
				skipPids[childPid] = true
			}
		}
	}

	// Apply filters if any are specified
	rootPids, pidsToShow := filterProcesses(pidToProcess, pidToChildren, skipPids, cli)

	// Sort root PIDs
	sort.Ints(rootPids)

	// Get terminal width
	termWidth := getTerminalWidth()

	// Calculate column widths
	maxUserLen := 10
	if cli.ShowFullUser {
		// Find actual max user length when showing full names
		for _, p := range pidToProcess {
			if len(truncateUser(p.User)) > maxUserLen {
				maxUserLen = len(truncateUser(p.User))
			}
		}
	}
	maxStartLen := 5
	maxTimeLen := 4
	
	for _, p := range pidToProcess {
		startStr := formatStartTime(p.StartTime)
		if len(startStr) > maxStartLen {
			maxStartLen = len(startStr)
		}
		timeStr := formatCPUTime(p.CPUTime)
		if len(strings.TrimSpace(timeStr)) > maxTimeLen {
			maxTimeLen = len(strings.TrimSpace(timeStr))
		}
	}
	
	// Ensure minimum width for TIME column
	if maxTimeLen < 8 {
		maxTimeLen = 8
	}

	// Print header  
	header := fmt.Sprintf("  %5s %-*s %5s %5s %5s   %-*s  %-*s  %s",
		centerText("PID", 5), maxUserLen, centerText("USER", maxUserLen), "%CPU", "%MEM", "RSS",
		maxStartLen, "START",
		maxTimeLen, centerText("TIME", maxTimeLen),
		"COMMAND")
	fmt.Println(header)
	if cli.ShowFullCommand {
		// When showing full commands, use a fixed width separator
		fmt.Println(strings.Repeat("-", 80))
	} else if termWidth > 0 {
		fmt.Println(strings.Repeat("-", termWidth))
	} else {
		// When piped without terminal, use a fixed width separator
		fmt.Println(strings.Repeat("-", 80))
	}

	// Print process trees
	// pidsToShow is only set when filters are applied
	
	for i, rootPid := range rootPids {
		isLast := i == len(rootPids)-1
		printProcessTree(os.Stdout, pidToProcess, pidToChildren, skipPids, pidsToShow, rootPid, "", isLast, maxUserLen, maxStartLen, maxTimeLen, termWidth)
	}
}


func printProcessTree(w io.Writer, processes map[int]*Process, children map[int][]int, skipPids map[int]bool, 
	pidsToShow map[int]bool, pid int, prefix string, isLast bool, maxUserLen, maxStartLen, maxTimeLen, termWidth int) {
	
	if skipPids[pid] {
		return
	}

	p, ok := processes[pid]
	if !ok {
		return
	}
	
	// Check if we should display this process
	shouldDisplay := pidsToShow == nil || pidsToShow[pid]

	// Format tree characters
	var branch string
	childPids := children[pid]
	hasChildren := false
	
	// Check if has non-skipped children
	// In filter mode, only count children that will be displayed
	for _, childPid := range childPids {
		if !skipPids[childPid] {
			if pidsToShow == nil || pidsToShow[childPid] {
				hasChildren = true
				break
			}
		}
	}
	
	if prefix == "" {
		if hasChildren {
			branch = "─┬─"
		} else {
			branch = "───"
		}
	} else if isLast {
		if hasChildren {
			branch = "└─┬─"
		} else {
			branch = "└───"
		}
	} else {
		if hasChildren {
			branch = "├─┬─"
		} else {
			branch = "├───"
		}
	}

	// Only print this process if it should be displayed
	if shouldDisplay {
		// Build the line
		treeStr := prefix + branch
		if branch != "" {
			treeStr = treeStr + " "
		}
		line := fmt.Sprintf("%7d %-*s %5.1f %5.1f %6s  %-*s  %-*s  %s%s",
			p.PID, maxUserLen, truncateUser(p.User),
			p.CPUPct, p.MemPct, formatRSS(p.RSSKB),
			maxStartLen, formatStartTime(p.StartTime),
			maxTimeLen, formatCPUTime(p.CPUTime),
			treeStr, p.Command)

		// Truncate if too long (UTF-8 aware)
		// Only truncate if we have a valid terminal width and not showing full commands
		if !cli.ShowFullCommand && termWidth > 0 && len(line) > termWidth && termWidth > 3 {
			// Convert to runes to handle UTF-8 properly
			runes := []rune(line)
			if len(runes) > termWidth-3 {
				line = string(runes[:termWidth-3]) + "..."
			}
		}

		fmt.Fprintln(w, line)
	}

	// Sort and print children
	sort.Ints(childPids)
	
	nonSkippedCount := 0
	for _, childPid := range childPids {
		if !skipPids[childPid] {
			nonSkippedCount++
		}
	}
	
	printed := 0
	for _, childPid := range childPids {
		if skipPids[childPid] {
			continue
		}
		
		printed++
		var childPrefix string
		if prefix == "" {
			childPrefix = " "
		} else if isLast {
			childPrefix = prefix + "  "
		} else {
			childPrefix = prefix + "│ "
		}
		
		isLastChild := printed == nonSkippedCount
		printProcessTree(w, processes, children, skipPids, pidsToShow, childPid, childPrefix, isLastChild, 
			maxUserLen, maxStartLen, maxTimeLen, termWidth)
	}
}


func centerText(text string, width int) string {
	padding := width - len(text)
	if padding <= 0 {
		return text
	}
	leftPad := padding / 2
	rightPad := padding - leftPad
	return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
}

func truncateUser(user string) string {
	if cli.ShowFullUser {
		return user
	}
	if len(user) <= 10 {
		return user
	}
	return user[:7] + "..."
}

func getTerminalWidth() int {
	termWidth := 80
	
	// First check COLUMNS environment variable
	if cols := os.Getenv("COLUMNS"); cols != "" {
		var w int
		if _, err := fmt.Sscanf(cols, "%d", &w); err == nil && w > 0 {
			termWidth = w
			return termWidth
		}
	}
	
	// Try to get terminal size from stdout
	if term.IsTerminal(int(os.Stdout.Fd())) {
		if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			termWidth = width
			return termWidth
		}
	}
	
	// If stdout is not a terminal (e.g., piped), try stdin
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if width, _, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
			termWidth = width
			return termWidth
		}
	}
	
	// If neither stdout nor stdin is a terminal, disable truncation
	if !term.IsTerminal(int(os.Stdout.Fd())) && !term.IsTerminal(int(os.Stdin.Fd())) {
		return 0
	}
	
	return termWidth
}

// formatRSS formats RSS in KB to a human-readable string
func formatRSS(rssKB float64) string {
	if rssKB >= 1048576 {
		return fmt.Sprintf("%.1fG", rssKB/1048576)
	}
	return fmt.Sprintf("%.1fM", rssKB/1024)
}

// formatStartTime formats start time for display
func formatStartTime(startTime *time.Time) string {
	if startTime == nil {
		return "--"
	}

	now := time.Now()
	ageHours := now.Sub(*startTime).Hours()

	if ageHours < 24 {
		// Less than 24 hours ago: show HH:MM
		return startTime.Format("15:04")
	} else if startTime.Year() == now.Year() {
		// Current year: show MonDD
		return startTime.Format("Jan02")
	} else {
		// Previous years: show YYYY
		return startTime.Format("2006")
	}
}

// formatCPUTime formats CPU time duration for display
func formatCPUTime(cpuTime time.Duration) string {
	if cpuTime == 0 {
		return "      --"
	}

	totalSeconds := int(cpuTime.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours >= 24 {
		// Right-justify the hours format
		return fmt.Sprintf("%5dhrs", hours)
	} else {
		// HH:MM:SS format for under 24 hours
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
}

// filterProcesses applies CLI filters and returns root PIDs and PIDs to show
func filterProcesses(pidToProcess map[int]*Process, pidToChildren map[int][]int, skipPids map[int]bool, filters CLI) ([]int, map[int]bool) {
	hasFilters := len(filters.PIDs) > 0 || len(filters.Users) > 0 || len(filters.SearchStrings) > 0 || len(filters.SearchStringsCase) > 0
	var rootPids []int
	var pidsToShow map[int]bool

	if hasFilters {
		matchingPids := findMatchingPids(pidToProcess, skipPids, filters)
		pidsToShow = expandToAncestorsAndDescendants(pidToProcess, pidToChildren, matchingPids)

		// Always start from true root processes (ppid = 0)
		// This ensures we get proper tree structure
		for pid, p := range pidToProcess {
			if p.PPID == 0 && pidsToShow[pid] {
				rootPids = append(rootPids, pid)
			}
		}
	} else {
		// No filters, show all from root
		// Find all root processes (those with ppid 0 or no parent in our set)
		for pid, p := range pidToProcess {
			if p.PPID == 0 || pidToProcess[p.PPID] == nil {
				rootPids = append(rootPids, pid)
			}
		}
	}

	return rootPids, pidsToShow
}

// findMatchingPids finds PIDs that match the given filters
func findMatchingPids(pidToProcess map[int]*Process, skipPids map[int]bool, filters CLI) map[int]bool {
	matchingPids := make(map[int]bool)
	
	for _, p := range pidToProcess {
		if skipPids[p.PID] {
			continue
		}

		// Check PID filters
		for _, pidStr := range filters.PIDs {
			if strconv.Itoa(p.PID) == pidStr {
				matchingPids[p.PID] = true
			}
		}

		// Check user filters
		for _, user := range filters.Users {
			if p.User == user {
				matchingPids[p.PID] = true
			}
		}

		// Check string filters
		for _, str := range filters.SearchStrings {
			if strings.Contains(p.Command, str) {
				matchingPids[p.PID] = true
			}
		}

		// Check case-insensitive string filters
		for _, str := range filters.SearchStringsCase {
			if strings.Contains(strings.ToLower(p.Command), strings.ToLower(str)) {
				matchingPids[p.PID] = true
			}
		}
	}
	
	return matchingPids
}

// expandToAncestorsAndDescendants expands matching PIDs to include all ancestors and descendants
func expandToAncestorsAndDescendants(pidToProcess map[int]*Process, pidToChildren map[int][]int, matchingPids map[int]bool) map[int]bool {
	pidsToShow := make(map[int]bool)
	
	for pid := range matchingPids {
		// Add the matching PID itself
		pidsToShow[pid] = true
		
		// Add all ancestors
		current := pid
		for {
			if p, ok := pidToProcess[current]; ok && p.PPID > 0 {
				pidsToShow[p.PPID] = true
				current = p.PPID
			} else {
				break
			}
		}
		
		// Add all descendants
		queue := []int{pid}
		visited := make(map[int]bool)
		visited[pid] = true
		
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			
			for _, child := range pidToChildren[current] {
				if !visited[child] {
					pidsToShow[child] = true
					queue = append(queue, child)
					visited[child] = true
				}
			}
		}
	}
	
	return pidsToShow
}

// parseUserArgs processes command-line arguments to handle -u/--user flag without argument
func parseUserArgs(args []string) ([]string, bool) {
	userFlagWithoutArg := false
	processedArgs := make([]string, len(args))
	copy(processedArgs, args)
	
	for i := 0; i < len(processedArgs); i++ {
		if processedArgs[i] == "-u" || processedArgs[i] == "--user" {
			// Check if next arg exists and is not another flag
			if i+1 >= len(processedArgs) || strings.HasPrefix(processedArgs[i+1], "-") {
				userFlagWithoutArg = true
				// Remove the -u/--user flag so Kong doesn't complain
				processedArgs = append(processedArgs[:i], processedArgs[i+1:]...)
				i--
			}
		}
	}
	
	return processedArgs, userFlagWithoutArg
}
