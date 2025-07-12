package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"golang.org/x/term"
)

type Process struct {
	pid   int
	ppid  int
	user  string
	pcpu  string
	pmem  string
	rss   string
	start string
	time  string
	cmd   string
}

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
	// Check for -u/--user flag without argument before Kong parses
	args := os.Args[1:]
	userFlagWithoutArg := false
	for i := 0; i < len(args); i++ {
		if args[i] == "-u" || args[i] == "--user" {
			// Check if next arg exists and is not another flag
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				userFlagWithoutArg = true
				// Remove the -u/--user flag so Kong doesn't complain
				args = append(args[:i], args[i+1:]...)
				i--
			}
		}
	}

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

	// Get all processes using ps
	processes := getAllProcesses()
	
	// Build process relationships
	pidToProcess := make(map[int]*Process)
	pidToChildren := make(map[int][]int)
	skipPids := make(map[int]bool)
	
	// Find proktree process to skip it and its children
	proktreePid := -1
	for i := range processes {
		p := &processes[i]
		pidToProcess[p.pid] = p
		if p.ppid > 0 {
			pidToChildren[p.ppid] = append(pidToChildren[p.ppid], p.pid)
		}
		
		// Mark proktree for skipping
		if strings.Contains(p.cmd, "proktree") {
			proktreePid = p.pid
			skipPids[p.pid] = true
		}
	}
	
	// Skip ps children of proktree
	if proktreePid > 0 {
		for _, childPid := range pidToChildren[proktreePid] {
			if p, ok := pidToProcess[childPid]; ok && strings.HasPrefix(p.cmd, "ps ") {
				skipPids[childPid] = true
			}
		}
	}

	// Apply filters if any are specified
	hasFilters := len(cli.PIDs) > 0 || len(cli.Users) > 0 || len(cli.SearchStrings) > 0 || len(cli.SearchStringsCase) > 0
	var rootPids []int
	var pidsToShow map[int]bool

	if hasFilters {
		matchingPids := make(map[int]bool)
		
		// Find matching PIDs
		for _, p := range processes {
			if skipPids[p.pid] {
				continue
			}

			// Check PID filters
			for _, pidStr := range cli.PIDs {
				if strconv.Itoa(p.pid) == pidStr {
					matchingPids[p.pid] = true
				}
			}

			// Check user filters
			for _, user := range cli.Users {
				if p.user == user {
					matchingPids[p.pid] = true
				}
			}

			// Check string filters
			for _, str := range cli.SearchStrings {
				if strings.Contains(p.cmd, str) {
					matchingPids[p.pid] = true
				}
			}

			// Check case-insensitive string filters
			for _, str := range cli.SearchStringsCase {
				if strings.Contains(strings.ToLower(p.cmd), strings.ToLower(str)) {
					matchingPids[p.pid] = true
				}
			}
		}

		// Find all ancestors and descendants
		pidsToShow = make(map[int]bool)
		
		for pid := range matchingPids {
			// Add the matching PID itself
			pidsToShow[pid] = true
			
			// Add all ancestors
			current := pid
			for {
				if p, ok := pidToProcess[current]; ok && p.ppid > 0 {
					pidsToShow[p.ppid] = true
					current = p.ppid
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

		// Always start from true root processes (ppid = 0)
		// This ensures we get proper tree structure
		for pid, p := range pidToProcess {
			if p.ppid == 0 && pidsToShow[pid] {
				rootPids = append(rootPids, pid)
			}
		}

		// Don't filter pidToProcess - we need all processes for tree traversal
		// Instead, pass pidsToShow to printProcessTree
	} else {
		// No filters, show all from root
		// Find all root processes (those with ppid 0 or no parent in our set)
		for pid, p := range pidToProcess {
			if p.ppid == 0 || pidToProcess[p.ppid] == nil {
				rootPids = append(rootPids, pid)
			}
		}
	}

	// Sort root PIDs
	sort.Ints(rootPids)

	// Get terminal width
	termWidth := getTerminalWidth()

	// Calculate column widths
	maxUserLen := 10
	if cli.ShowFullUser {
		// Find actual max user length when showing full names
		for _, p := range pidToProcess {
			if len(p.user) > maxUserLen {
				maxUserLen = len(p.user)
			}
		}
	}
	maxStartLen := 5
	maxTimeLen := 4
	
	for _, p := range pidToProcess {
		if len(p.start) > maxStartLen {
			maxStartLen = len(p.start)
		}
		if len(strings.TrimSpace(p.time)) > maxTimeLen {
			maxTimeLen = len(strings.TrimSpace(p.time))
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
	var pidsToShowMap map[int]bool
	if hasFilters {
		pidsToShowMap = pidsToShow
	}
	
	for i, rootPid := range rootPids {
		isLast := i == len(rootPids)-1
		printProcessTree(pidToProcess, pidToChildren, skipPids, pidsToShowMap, rootPid, "", isLast, maxUserLen, maxStartLen, maxTimeLen, termWidth)
	}
}

func getAllProcesses() []Process {
	// Get process info including PPID
	cmd := exec.Command("ps", "-axo", "pid,ppid,user,pcpu,pmem,rss,lstart,time,command")
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run ps: %v\n", err)
		os.Exit(1)
	}

	var processes []Process
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Scan() // Skip header

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		
		if len(fields) < 9 {
			continue
		}

		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		user := truncateUser(fields[2])
		pcpu := fields[3]
		pmem := fields[4]
		rssKb, _ := strconv.ParseFloat(fields[5], 64)
		
		// Format RSS
		var rssFmt string
		if rssKb >= 1048576 {
			rssFmt = fmt.Sprintf("%.1fG", rssKb/1048576)
		} else {
			rssFmt = fmt.Sprintf("%.1fM", rssKb/1024)
		}

		// Parse lstart and time
		// lstart format: "Thu Jul 10 15:37:36 2025" (6 fields)
		// After that comes the TIME field, then COMMAND
		var startRaw string
		var timeStr string
		var cmd string
		
		// Find where TIME field starts (after year in lstart)
		// lstart is 6 fields starting at field 6
		if len(fields) >= 13 {
			// Standard format: fields 6-11 are lstart (Thu Jul 10 15:37:36 2025)
			// field 12 is TIME
			// field 13+ is COMMAND
			startRaw = strings.Join(fields[6:11], " ") // Don't include the time field
			timeStr = fields[11]
			cmd = strings.Join(fields[12:], " ")
		} else {
			// Fallback for unexpected format
			startRaw = "--"
			timeStr = "--"
			cmd = strings.Join(fields[8:], " ")
		}

		// Format START
		startFmt := formatStartTime(startRaw)

		// Format TIME
		timeFmt := formatCPUTime(timeStr)

		processes = append(processes, Process{
			pid:   pid,
			ppid:  ppid,
			user:  user,
			pcpu:  pcpu,
			pmem:  pmem,
			rss:   rssFmt,
			start: startFmt,
			time:  timeFmt,
			cmd:   cmd,
		})
	}

	return processes
}

func printProcessTree(processes map[int]*Process, children map[int][]int, skipPids map[int]bool, 
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
		line := fmt.Sprintf("%7d %-*s %5s %5s %6s  %-*s  %-*s  %s%s",
			p.pid, maxUserLen, p.user,
			p.pcpu, p.pmem, p.rss,
			maxStartLen, p.start,
			maxTimeLen, p.time,
			treeStr, p.cmd)

		// Truncate if too long (UTF-8 aware)
		// Only truncate if we have a valid terminal width and not showing full commands
		if !cli.ShowFullCommand && termWidth > 0 && len(line) > termWidth && termWidth > 3 {
			// Convert to runes to handle UTF-8 properly
			runes := []rune(line)
			if len(runes) > termWidth-3 {
				line = string(runes[:termWidth-3]) + "..."
			}
		}

		fmt.Println(line)
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
		printProcessTree(processes, children, skipPids, pidsToShow, childPid, childPrefix, isLastChild, 
			maxUserLen, maxStartLen, maxTimeLen, termWidth)
	}
}

func formatStartTime(startRaw string) string {
	if startRaw == "--" {
		return "--"
	}

	// Try various date formats that ps might use
	// Format from ps: "Thu Jul 10 15:37:36 2025"
	formats := []string{
		"Mon Jan _2 15:04:05 2006",  // Single digit day with padding
		"Mon Jan  2 15:04:05 2006",  // Double space before single digit
		"Mon Jan 2 15:04:05 2006",   // Single space
		"Mon Jan 02 15:04:05 2006",  // Zero-padded day
	}
	
	var t time.Time
	var err error
	
	// First try to parse as-is
	for _, format := range formats {
		t, err = time.Parse(format, startRaw)
		if err == nil {
			break
		}
	}
	
	if err != nil {
		return "--"
	}

	now := time.Now()
	ageHours := now.Sub(t).Hours()

	if ageHours < 24 {
		// Less than 24 hours ago: show HH:MM
		return t.Format("15:04")
	} else if t.Year() == now.Year() {
		// Current year: show MonDD
		return t.Format("Jan02")
	} else {
		// Previous years: show YYYY
		return t.Format("2006")
	}
}

func formatCPUTime(timeStr string) string {
	// Parse time format (can be M:SS.ss or HH:MM.ss)
	parts := strings.Split(timeStr, ":")
	if len(parts) == 2 {
		// Check if it's HH:MM.ss or M:SS.ss
		if strings.Contains(parts[1], ".") && len(strings.Split(parts[1], ".")[0]) == 2 {
			// HH:MM.ss format
			hours, _ := strconv.Atoi(parts[0])
			if hours >= 24 {
				return fmt.Sprintf(" %dhrs ", hours)
			}
			return fmt.Sprintf("%2s:%s", parts[0], parts[1])
		}
		// M:SS.ss format
		return fmt.Sprintf("%s:%s", parts[0], parts[1])
	}
	return timeStr
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
