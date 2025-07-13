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

// DefaultScreenWidth is the fallback when terminal width cannot be determined
const DefaultScreenWidth = 80

// Command-line args
type CLI struct {
	PIDs              []string `short:"p" name:"pid" help:"Show only parents and descendants of process PID (can be specified multiple times)"`
	Users             []string `short:"u" name:"user" help:"Show only parents and descendants of processes of USER (can be specified multiple times, defaults to current user if -u is used without argument)"`
	SearchStrings     []string `short:"s" name:"string" help:"Show only parents and descendants of process names containing STRING (can be specified multiple times)"`
	SearchStringsCase []string `short:"i" name:"string-insensitive" help:"Show only parents and descendants of process names containing STRING case-insensitively (can be specified multiple times)"`
	ShowFullUser      bool     `name:"long-users" help:"Show full usernames, without truncation"`
	ShowFullCommand   bool     `name:"long-commands" help:"Show full commands, without truncation"`
}

// Main comms
type Proktree struct {
	processes   map[int]*Process
	children    map[int][]int
	skipPids    map[int]bool
	pidsToShow  map[int]bool
	rootPids    []int
	maxUserLen  int
	maxStartLen int
	maxTimeLen  int
	termWidth   int
	cli         CLI
}

func main() {
	pt := &Proktree{
		cli:       CLI{},
		processes: make(map[int]*Process),
		children:  make(map[int][]int),
		skipPids:  make(map[int]bool),
		termWidth: getTerminalWidth(),
	}

	// Parse command-line arguments
	args, userFlagWithoutArg := parseUserArgs(os.Args[1:])

	// Parse with modified args
	os.Args = append([]string{os.Args[0]}, args...)
	_ = kong.Parse(&pt.cli,
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
			pt.cli.Users = append(pt.cli.Users, currentUser.Username)
		}
	}

	// Get all processes
	platform := GetPlatform()
	processList, err := platform.GetProcesses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get processes: %v\n", err)
		os.Exit(1)
	}

	pt.buildProcessRelationships(processList)
	pt.applyFilters()
	pt.calculateColumnWidths()
	pt.printHeader(os.Stdout)
	pt.printTrees(os.Stdout)
}

// buildProcessRelationships builds parent-child relationships
func (pt *Proktree) buildProcessRelationships(processList []Process) {
	proktreePid := -1
	for i := range processList {
		p := &processList[i]
		pt.processes[p.PID] = p
		if p.PPID > 0 {
			pt.children[p.PPID] = append(pt.children[p.PPID], p.PID)
		}

		// Mark proktree for skipping
		if strings.Contains(p.Command, "proktree") {
			proktreePid = p.PID
			pt.skipPids[p.PID] = true
		}
	}

	// Store for ps children skipping
	if proktreePid > 0 {
		for _, childPid := range pt.children[proktreePid] {
			if p, ok := pt.processes[childPid]; ok && strings.HasPrefix(p.Command, "ps ") {
				pt.skipPids[childPid] = true
			}
		}
	}
}

// applyFilters applies CLI filters to determine which processes to show
func (pt *Proktree) applyFilters() {
	pt.rootPids, pt.pidsToShow = pt.filterProcesses()
	sort.Ints(pt.rootPids)
}

// calculateColumnWidths calculates the maximum width for variable columns
func (pt *Proktree) calculateColumnWidths() {
	pt.maxUserLen = 10
	if pt.cli.ShowFullUser {
		// Find actual max user length when showing full names
		for _, p := range pt.processes {
			if len(p.User) > pt.maxUserLen {
				pt.maxUserLen = len(p.User)
			}
		}
	}
	pt.maxStartLen = 5
	pt.maxTimeLen = 4

	for _, p := range pt.processes {
		startStr := formatStartTime(p.StartTime)
		if len(startStr) > pt.maxStartLen {
			pt.maxStartLen = len(startStr)
		}
		timeStr := formatCPUTime(p.CPUTime)
		if len(strings.TrimSpace(timeStr)) > pt.maxTimeLen {
			pt.maxTimeLen = len(strings.TrimSpace(timeStr))
		}
	}

	// Ensure minimum width for TIME column
	if pt.maxTimeLen < 8 {
		pt.maxTimeLen = 8
	}
}

// printHeader prints the column headers
func (pt *Proktree) printHeader(w io.Writer) {
	header := fmt.Sprintf("  %5s %-*s %5s %5s %5s   %-*s  %-*s  %s",
		centerText("PID", 5), pt.maxUserLen, centerText("USER", pt.maxUserLen), "%CPU", "%MEM", "RSS",
		pt.maxStartLen, "START",
		pt.maxTimeLen, centerText("TIME", pt.maxTimeLen),
		"COMMAND")
	fmt.Fprintln(w, header)
	if pt.cli.ShowFullCommand {
		// When showing full commands, use a fixed width separator
		fmt.Fprintln(w, strings.Repeat("-", DefaultScreenWidth))
	} else if pt.termWidth > 0 {
		fmt.Fprintln(w, strings.Repeat("-", pt.termWidth))
	} else {
		// When piped without terminal, use a fixed width separator
		fmt.Fprintln(w, strings.Repeat("-", DefaultScreenWidth))
	}
}

// printTrees prints all process trees
func (pt *Proktree) printTrees(w io.Writer) {
	for i, rootPid := range pt.rootPids {
		isLast := i == len(pt.rootPids)-1
		pt.printProcessTree(w, rootPid, isLast)
	}
}

// processLine represents a buffered output line with tree metadata
type processLine struct {
	pid                int
	depth              int
	isLast             bool
	hasVisibleChildren bool
	content            string // The formatted process info without tree graphics
	prefix             string // The full tree prefix including indentation
}

// collectProcessLines collects all process lines that should be displayed
func (pt *Proktree) collectProcessLines(pid int, depth int, prefix string, isLast bool) []processLine {

	if pt.skipPids[pid] {
		return nil
	}

	p, ok := pt.processes[pid]
	if !ok {
		return nil
	}

	// Check if we should display this process
	shouldDisplay := pt.pidsToShow == nil || pt.pidsToShow[pid]
	if !shouldDisplay {
		// Still need to collect children
		var lines []processLine
		childPids := pt.children[pid]
		sort.Ints(childPids)

		visibleChildren := 0
		for _, childPid := range childPids {
			if !pt.skipPids[childPid] && (pt.pidsToShow == nil || pt.pidsToShow[childPid]) {
				visibleChildren++
			}
		}

		childIdx := 0
		for _, childPid := range childPids {
			if pt.skipPids[childPid] {
				continue
			}

			// Determine child prefix based on whether THIS process is last
			var childPrefix string
			if depth == 0 {
				// Root's children start with no prefix
				childPrefix = ""
			} else if isLast {
				// If this process is last, children get spaces
				childPrefix = prefix + "  "
			} else {
				// If this process is not last, children get a vertical line
				childPrefix = prefix + "│ "
			}

			if pt.pidsToShow != nil && !pt.pidsToShow[childPid] {
				// Need to check if this child has visible descendants
				childLines := pt.collectProcessLines(childPid, depth+1, childPrefix, false)
				if len(childLines) > 0 {
					lines = append(lines, childLines...)
				}
				continue
			}

			childIdx++
			isLastChild := childIdx == visibleChildren
			childLines := pt.collectProcessLines(childPid, depth+1, childPrefix, isLastChild)
			lines = append(lines, childLines...)
		}
		return lines
	}

	// Format the process info
	content := fmt.Sprintf("%7d %-*s %5.1f %5.1f %6s  %-*s  %-*s",
		p.PID, pt.maxUserLen, pt.truncateUser(p.User),
		p.CPUPct, p.MemPct, formatRSS(p.RSSKB),
		pt.maxStartLen, formatStartTime(p.StartTime),
		pt.maxTimeLen, formatCPUTime(p.CPUTime))

	// Check if has visible children
	childPids := pt.children[pid]
	hasVisibleChildren := false
	for _, childPid := range childPids {
		if !pt.skipPids[childPid] && (pt.pidsToShow == nil || pt.pidsToShow[childPid]) {
			hasVisibleChildren = true
			break
		}
	}

	line := processLine{
		pid:                pid,
		depth:              depth,
		prefix:             prefix,
		isLast:             isLast,
		hasVisibleChildren: hasVisibleChildren,
		content:            content,
	}

	lines := []processLine{line}

	// Collect children
	sort.Ints(childPids)

	visibleChildren := 0
	for _, childPid := range childPids {
		if !pt.skipPids[childPid] && (pt.pidsToShow == nil || pt.pidsToShow[childPid]) {
			visibleChildren++
		}
	}

	childIdx := 0
	for _, childPid := range childPids {
		if pt.skipPids[childPid] {
			continue
		}

		// Determine child prefix based on whether THIS process is last
		var childPrefix string
		if depth == 0 {
			// Root's children start with no prefix
			childPrefix = ""
		} else if isLast {
			// If this process is last, children get spaces
			childPrefix = prefix + "  "
		} else {
			// If this process is not last, children get a vertical line
			childPrefix = prefix + "│ "
		}

		if pt.pidsToShow != nil && !pt.pidsToShow[childPid] {
			// Need to check if this child has visible descendants
			childLines := pt.collectProcessLines(childPid, depth+1, childPrefix, false)
			if len(childLines) > 0 {
				lines = append(lines, childLines...)
			}
			continue
		}

		childIdx++
		isLastChild := childIdx == visibleChildren
		childLines := pt.collectProcessLines(childPid, depth+1, childPrefix, isLastChild)
		lines = append(lines, childLines...)
	}

	return lines
}

// renderProcessTree renders the collected lines with optimized tree graphics
func (pt *Proktree) renderProcessTree(w io.Writer, lines []processLine) {
	// Render each line
	for _, line := range lines {
		// Determine the branch characters
		var branch string
		if line.depth == 0 {
			if line.hasVisibleChildren {
				branch = "─┬─"
			} else {
				branch = "───"
			}
		} else if line.isLast {
			if line.hasVisibleChildren {
				branch = "└─┬─"
			} else {
				branch = "└───"
			}
		} else {
			if line.hasVisibleChildren {
				branch = "├─┬─"
			} else {
				branch = "├───"
			}
		}

		// Get the command
		p := pt.processes[line.pid]

		// Build the full line with proper tree alignment
		treeStr := line.prefix + branch
		// Root needs 2 spaces, others need 3 for proper alignment
		spacing := "   "
		if line.depth == 0 {
			spacing = "  "
		}
		fullLine := fmt.Sprintf("%s%s%s %s", line.content, spacing, treeStr, p.Command)

		// Truncate if too long
		if !pt.cli.ShowFullCommand && pt.termWidth > 0 && len(fullLine) > pt.termWidth && pt.termWidth > 3 {
			runes := []rune(fullLine)
			if len(runes) > pt.termWidth-3 {
				fullLine = string(runes[:pt.termWidth-3]) + "..."
			}
		}

		fmt.Fprintln(w, fullLine)
	}
}

// printProcessTree prints a process tree starting from the given PID
func (pt *Proktree) printProcessTree(w io.Writer, pid int, isLast bool) {
	// Collect all lines
	lines := pt.collectProcessLines(pid, 0, "", isLast)

	// Render with optimized tree graphics
	pt.renderProcessTree(w, lines)
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

// truncateUser truncates usernames based on CLI settings
func (pt *Proktree) truncateUser(user string) string {
	if pt.cli.ShowFullUser {
		return user
	}
	if len(user) <= 10 {
		return user
	}
	return user[:7] + "..."
}

func getTerminalWidth() int {
	termWidth := DefaultScreenWidth

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
func (pt *Proktree) filterProcesses() ([]int, map[int]bool) {
	hasFilters := len(pt.cli.PIDs) > 0 || len(pt.cli.Users) > 0 || len(pt.cli.SearchStrings) > 0 || len(pt.cli.SearchStringsCase) > 0
	var rootPids []int
	var pidsToShow map[int]bool

	if hasFilters {
		matchingPids := pt.findMatchingPids()
		pidsToShow = pt.expandToAncestorsAndDescendants(matchingPids)

		// Always start from true root processes (ppid = 0)
		// This ensures we get proper tree structure
		for pid, p := range pt.processes {
			if p.PPID == 0 && pidsToShow[pid] {
				rootPids = append(rootPids, pid)
			}
		}
	} else {
		// No filters, show all from root
		// Find all root processes (those with ppid 0 or no parent in our set)
		for pid, p := range pt.processes {
			if p.PPID == 0 || pt.processes[p.PPID] == nil {
				rootPids = append(rootPids, pid)
			}
		}
	}

	return rootPids, pidsToShow
}

// findMatchingPids finds PIDs that match the given filters
func (pt *Proktree) findMatchingPids() map[int]bool {
	matchingPids := make(map[int]bool)

	for _, p := range pt.processes {
		if pt.skipPids[p.PID] {
			continue
		}

		// Check PID filters
		for _, pidStr := range pt.cli.PIDs {
			if strconv.Itoa(p.PID) == pidStr {
				matchingPids[p.PID] = true
			}
		}

		// Check user filters
		for _, user := range pt.cli.Users {
			if p.User == user {
				matchingPids[p.PID] = true
			}
		}

		// Check string filters
		for _, str := range pt.cli.SearchStrings {
			if strings.Contains(p.Command, str) {
				matchingPids[p.PID] = true
			}
		}

		// Check case-insensitive string filters
		for _, str := range pt.cli.SearchStringsCase {
			if strings.Contains(strings.ToLower(p.Command), strings.ToLower(str)) {
				matchingPids[p.PID] = true
			}
		}
	}

	return matchingPids
}

// expandToAncestorsAndDescendants expands matching PIDs to include all ancestors and descendants
func (pt *Proktree) expandToAncestorsAndDescendants(matchingPids map[int]bool) map[int]bool {
	pidsToShow := make(map[int]bool)

	for pid := range matchingPids {
		// Add the matching PID itself
		pidsToShow[pid] = true

		// Add all ancestors
		current := pid
		for {
			if p, ok := pt.processes[current]; ok && p.PPID > 0 {
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

			for _, child := range pt.children[current] {
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
