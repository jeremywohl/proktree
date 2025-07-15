package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestFormatStartTime(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		input    *time.Time
		expected string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: "--",
		},
		{
			name:     "recent time (less than 24 hours)",
			input:    func() *time.Time { t := now.Add(-2 * time.Hour); return &t }(),
			expected: now.Add(-2 * time.Hour).Format("15:04"),
		},
		{
			name:     "current year",
			input:    func() *time.Time { t := now.Add(-48 * time.Hour); return &t }(),
			expected: now.Add(-48 * time.Hour).Format("Jan02"),
		},
		{
			name:     "previous year",
			input:    func() *time.Time { t := now.Add(-400 * 24 * time.Hour); return &t }(),
			expected: now.Add(-400 * 24 * time.Hour).Format("2006"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStartTime(tt.input)
			if result != tt.expected {
				t.Errorf("formatStartTime(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatCPUTime(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			input:    0,
			expected: "      --",
		},
		{
			name:     "minutes and seconds",
			input:    1*time.Minute + 23*time.Second + 450*time.Millisecond,
			expected: "00:01:23",
		},
		{
			name:     "hours and minutes",
			input:    12*time.Hour + 34*time.Minute + 56*time.Second,
			expected: "12:34:56",
		},
		{
			name:     "24+ hours",
			input:    25 * time.Hour,
			expected: "   25hrs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatCPUTime(tt.input)
			if result != tt.expected {
				t.Errorf("formatCPUTime(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCenterText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		width    int
		expected string
	}{
		{
			name:     "text shorter than width",
			text:     "PID",
			width:    5,
			expected: " PID ",
		},
		{
			name:     "text equal to width",
			text:     "HELLO",
			width:    5,
			expected: "HELLO",
		},
		{
			name:     "text longer than width",
			text:     "TOOLONG",
			width:    5,
			expected: "TOOLONG",
		},
		{
			name:     "even padding",
			text:     "HI",
			width:    6,
			expected: "  HI  ",
		},
		{
			name:     "odd padding",
			text:     "HI",
			width:    5,
			expected: " HI  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := centerText(tt.text, tt.width)
			if result != tt.expected {
				t.Errorf("centerText(%q, %d) = %q, want %q", tt.text, tt.width, result, tt.expected)
			}
		})
	}
}

func TestTruncateUser(t *testing.T) {

	tests := []struct {
		name         string
		user         string
		showFullUser bool
		expected     string
	}{
		{
			name:         "short username",
			user:         "root",
			showFullUser: false,
			expected:     "root",
		},
		{
			name:         "long username truncated",
			user:         "verylongusername",
			showFullUser: false,
			expected:     "verylon...",
		},
		{
			name:         "long username full",
			user:         "verylongusername",
			showFullUser: true,
			expected:     "verylongusername",
		},
		{
			name:         "exactly 10 chars",
			user:         "1234567890",
			showFullUser: false,
			expected:     "1234567890",
		},
		{
			name:         "11 chars truncated",
			user:         "12345678901",
			showFullUser: false,
			expected:     "1234567...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := &Proktree{
				cli: CLI{ShowFullUser: tt.showFullUser},
			}
			result := pt.truncateUser(tt.user)
			if result != tt.expected {
				t.Errorf("truncateUser(%q) = %q, want %q", tt.user, result, tt.expected)
			}
		})
	}
}

func TestGetTerminalWidth(t *testing.T) {
	originalColumns := os.Getenv("COLUMNS")
	defer os.Setenv("COLUMNS", originalColumns)

	tests := []struct {
		name     string
		setup    func()
		expected int
		checkFn  func(int) bool
	}{
		{
			name: "COLUMNS env var set",
			setup: func() {
				os.Setenv("COLUMNS", "120")
			},
			expected: 120,
		},
		{
			name: "COLUMNS env var invalid",
			setup: func() {
				os.Setenv("COLUMNS", "invalid")
			},
			checkFn: func(w int) bool {
				return w >= 0
			},
		},
		{
			name: "COLUMNS env var empty",
			setup: func() {
				os.Unsetenv("COLUMNS")
			},
			checkFn: func(w int) bool {
				return w >= 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := getTerminalWidth()
			if tt.checkFn != nil {
				if !tt.checkFn(result) {
					t.Errorf("getTerminalWidth() = %d, check failed", result)
				}
			} else if result != tt.expected {
				t.Errorf("getTerminalWidth() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestProcessFiltering(t *testing.T) {
	processes := map[int]*Process{
		1: {PID: 1, PPID: 0, User: "root", Command: "init"},
		2: {PID: 2, PPID: 1, User: "root", Command: "kernel_task"},
		3: {PID: 3, PPID: 1, User: "daemon", Command: "systemd"},
		4: {PID: 4, PPID: 3, User: "daemon", Command: "cron"},
		5: {PID: 5, PPID: 3, User: "user1", Command: "bash"},
		6: {PID: 6, PPID: 5, User: "user1", Command: "vim test.txt"},
	}

	pidToChildren := map[int][]int{
		1: {2, 3},
		3: {4, 5},
		5: {6},
	}

	tests := []struct {
		name             string
		cli              CLI
		expectedPidsShow map[int]bool
		expectedRootPids []int
	}{
		{
			name: "filter by PID",
			cli: CLI{
				PIDs: []string{"5"},
			},
			expectedPidsShow: map[int]bool{
				1: true, // ancestor
				3: true, // ancestor
				5: true, // matched
				6: true, // descendant
			},
			expectedRootPids: []int{1},
		},
		{
			name: "filter by user",
			cli: CLI{
				Users: []string{"daemon"},
			},
			expectedPidsShow: map[int]bool{
				1: true, // ancestor
				3: true, // matched
				4: true, // matched (child of 3, also daemon)
				5: true, // descendant of matched
				6: true, // descendant of matched
			},
			expectedRootPids: []int{1},
		},
		{
			name: "filter by string",
			cli: CLI{
				SearchStrings: []string{"vim"},
			},
			expectedPidsShow: map[int]bool{
				1: true, // ancestor
				3: true, // ancestor
				5: true, // ancestor
				6: true, // matched
			},
			expectedRootPids: []int{1},
		},
		{
			name: "filter by case-insensitive string",
			cli: CLI{
				SearchStringsCase: []string{"VIM"},
			},
			expectedPidsShow: map[int]bool{
				1: true, // ancestor
				3: true, // ancestor
				5: true, // ancestor
				6: true, // matched
			},
			expectedRootPids: []int{1},
		},
		{
			name: "multiple filters",
			cli: CLI{
				Users:         []string{"user1"},
				SearchStrings: []string{"bash"},
			},
			expectedPidsShow: map[int]bool{
				1: true, // ancestor
				3: true, // ancestor
				5: true, // matched both
				6: true, // descendant
			},
			expectedRootPids: []int{1},
		},
		{
			name:             "no filters",
			cli:              CLI{},
			expectedPidsShow: nil,      // No filtering, so pidsToShow should be nil
			expectedRootPids: []int{1}, // Root process
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipPids := make(map[int]bool)
			pt := &Proktree{
				processes: processes,
				children:  pidToChildren,
				skipPids:  skipPids,
				cli:       tt.cli,
			}
			rootPids, pidsToShow := pt.filterProcesses()

			// Check root PIDs
			if !equalIntSlices(rootPids, tt.expectedRootPids) {
				t.Errorf("rootPids = %v, want %v", rootPids, tt.expectedRootPids)
			}

			// Check PIDs to show
			if tt.expectedPidsShow == nil {
				if pidsToShow != nil {
					t.Errorf("pidsToShow = %v, want nil", pidsToShow)
				}
			} else {
				for pid, expected := range tt.expectedPidsShow {
					if pidsToShow[pid] != expected {
						t.Errorf("PID %d: got %v, want %v", pid, pidsToShow[pid], expected)
					}
				}
				// Also check that no unexpected PIDs are included
				for pid, included := range pidsToShow {
					if included && !tt.expectedPidsShow[pid] {
						t.Errorf("PID %d included but not expected", pid)
					}
				}
			}
		})
	}
}

// Helper function to compare int slices
func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestProcessTreeOutput(t *testing.T) {
	// Create test processes with static times
	jul10 := time.Date(2025, 7, 10, 0, 0, 0, 0, time.UTC)     // Current year -> "Jul10"
	jun01 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)      // Current year -> "Jun01"
	recent1 := time.Date(2025, 7, 15, 13, 10, 0, 0, time.UTC) // Within 24h -> "13:10"
	recent2 := time.Date(2025, 7, 15, 14, 40, 0, 0, time.UTC) // Within 24h -> "14:40"
	lastYear := time.Date(2023, 12, 25, 0, 0, 0, 0, time.UTC) // Previous year -> "2023"

	processes := map[int]*Process{
		1: {
			PID:       1,
			PPID:      0,
			User:      "root",
			CPUPct:    1.5,
			MemPct:    0.8,
			RSSKB:     31641.6, // 30.9M
			StartTime: &jul10,
			CPUTime:   28*time.Minute + 35*time.Second,
			Command:   "/sbin/launchd",
		},
		100: {
			PID:       100,
			PPID:      1,
			User:      "daemon",
			CPUPct:    0.0,
			MemPct:    0.1,
			RSSKB:     10240.0, // 10.0M
			StartTime: &jul10,
			CPUTime:   0,
			Command:   "/usr/sbin/sshd",
		},
		200: {
			PID:       200,
			PPID:      100,
			User:      "alice",
			CPUPct:    25.3,
			MemPct:    15.2,
			RSSKB:     1048576.0, // 1.0G
			StartTime: nil,
			CPUTime:   25 * time.Hour,
			Command:   "sshd: alice [priv]",
		},
		201: {
			PID:       201,
			PPID:      200,
			User:      "alice",
			CPUPct:    0.1,
			MemPct:    0.5,
			RSSKB:     5120.0, // 5.0M
			StartTime: &recent1,
			CPUTime:   2*time.Minute + 15*time.Second,
			Command:   "-bash",
		},
		300: {
			PID:       300,
			PPID:      1,
			User:      "postgres",
			CPUPct:    5.2,
			MemPct:    12.3,
			RSSKB:     524288.0, // 512M
			StartTime: &jun01,
			CPUTime:   125 * time.Hour,
			Command:   "/usr/bin/postgres -D /var/lib/postgresql",
		},
		301: {
			PID:       301,
			PPID:      300,
			User:      "postgres",
			CPUPct:    0.5,
			MemPct:    2.1,
			RSSKB:     102400.0, // 100M
			StartTime: &jun01,
			CPUTime:   45*time.Minute + 30*time.Second,
			Command:   "postgres: writer process",
		},
		302: {
			PID:       302,
			PPID:      300,
			User:      "postgres",
			CPUPct:    0.3,
			MemPct:    1.8,
			RSSKB:     81920.0, // 80M
			StartTime: &jun01,
			CPUTime:   22 * time.Minute,
			Command:   "postgres: checkpointer",
		},
		400: {
			PID:       400,
			PPID:      1,
			User:      "bob",
			CPUPct:    15.7,
			MemPct:    8.9,
			RSSKB:     204800.0, // 200M
			StartTime: &recent2,
			CPUTime:   5*time.Minute + 45*time.Second,
			Command:   "node server.js",
		},
		500: {
			PID:       500,
			PPID:      1,
			User:      "verylongusername",
			CPUPct:    0.0,
			MemPct:    0.1,
			RSSKB:     2048.0, // 2.0M
			StartTime: &lastYear,
			CPUTime:   0,
			Command:   "/usr/local/bin/custom-daemon",
		},
	}

	pidToChildren := map[int][]int{
		1:   {100, 300, 400, 500},
		100: {200},
		200: {201},
		300: {301, 302},
	}

	skipPids := make(map[int]bool)

	tests := []struct {
		name         string
		cli          CLI
		maxUserLen   int
		showFullUser bool
		expected     []string // Expected output lines
	}{
		{
			name:       "no filter - show all",
			cli:        CLI{},
			maxUserLen: 10,
			expected: []string{
				"   PID     USER     %CPU  %MEM   RSS   START    TIME    COMMAND",
				"--------------------------------------------------------------------------------",
				"      1 root         1.5   0.8  30.9M  Jul10  00:28:35  ─┬─ /sbin/launchd",
				"    100 daemon       0.0   0.1  10.0M  Jul10        --   ├─┬─ /usr/sbin/sshd",
				"    200 alice       25.3  15.2   1.0G  --        25hrs   │ └─┬─ sshd: alice [priv]",
				"    201 alice        0.1   0.5   5.0M  13:10  00:02:15   │   └─── -bash",
				"    300 postgres     5.2  12.3 512.0M  Jun01    125hrs   ├─┬─ /usr/bin/postgres -D /var/lib/postgresql",
				"    301 postgres     0.5   2.1 100.0M  Jun01  00:45:30   │ ├─── postgres: writer process",
				"    302 postgres     0.3   1.8  80.0M  Jun01  00:22:00   │ └─── postgres: checkpointer",
				"    400 bob         15.7   8.9 200.0M  14:40  00:05:45   ├─── node server.js",
				"    500 verylon...   0.0   0.1   2.0M  2023         --   └─── /usr/local/bin/custom-daemon",
			},
		},
		{
			name:       "filter by PID - shows ancestors and descendants",
			cli:        CLI{PIDs: []string{"200"}},
			maxUserLen: 10,
			expected: []string{
				"   PID     USER     %CPU  %MEM   RSS   START    TIME    COMMAND",
				"--------------------------------------------------------------------------------",
				"      1 root         1.5   0.8  30.9M  Jul10  00:28:35  ─┬─ /sbin/launchd",
				"    100 daemon       0.0   0.1  10.0M  Jul10        --   └─┬─ /usr/sbin/sshd",
				"    200 alice       25.3  15.2   1.0G  --        25hrs     └─┬─ sshd: alice [priv]",
				"    201 alice        0.1   0.5   5.0M  13:10  00:02:15       └─── -bash",
			},
		},
		{
			name:       "filter by user alice",
			cli:        CLI{Users: []string{"alice"}},
			maxUserLen: 10,
			expected: []string{
				"   PID     USER     %CPU  %MEM   RSS   START    TIME    COMMAND",
				"--------------------------------------------------------------------------------",
				"      1 root         1.5   0.8  30.9M  Jul10  00:28:35  ─┬─ /sbin/launchd",
				"    100 daemon       0.0   0.1  10.0M  Jul10        --   └─┬─ /usr/sbin/sshd",
				"    200 alice       25.3  15.2   1.0G  --        25hrs     └─┬─ sshd: alice [priv]",
				"    201 alice        0.1   0.5   5.0M  13:10  00:02:15       └─── -bash",
			},
		},
		{
			name:       "filter by user postgres",
			cli:        CLI{Users: []string{"postgres"}},
			maxUserLen: 10,
			expected: []string{
				"   PID     USER     %CPU  %MEM   RSS   START    TIME    COMMAND",
				"--------------------------------------------------------------------------------",
				"      1 root         1.5   0.8  30.9M  Jul10  00:28:35  ─┬─ /sbin/launchd",
				"    300 postgres     5.2  12.3 512.0M  Jun01    125hrs   └─┬─ /usr/bin/postgres -D /var/lib/postgresql",
				"    301 postgres     0.5   2.1 100.0M  Jun01  00:45:30     ├─── postgres: writer process",
				"    302 postgres     0.3   1.8  80.0M  Jun01  00:22:00     └─── postgres: checkpointer",
			},
		},
		{
			name:       "filter by command postgres",
			cli:        CLI{SearchStrings: []string{"postgres"}},
			maxUserLen: 10,
			expected: []string{
				"   PID     USER     %CPU  %MEM   RSS   START    TIME    COMMAND",
				"--------------------------------------------------------------------------------",
				"      1 root         1.5   0.8  30.9M  Jul10  00:28:35  ─┬─ /sbin/launchd",
				"    300 postgres     5.2  12.3 512.0M  Jun01    125hrs   └─┬─ /usr/bin/postgres -D /var/lib/postgresql",
				"    301 postgres     0.5   2.1 100.0M  Jun01  00:45:30     ├─── postgres: writer process",
				"    302 postgres     0.3   1.8  80.0M  Jun01  00:22:00     └─── postgres: checkpointer",
			},
		},
		{
			name:       "filter by multiple users",
			cli:        CLI{Users: []string{"alice", "bob"}},
			maxUserLen: 10,
			expected: []string{
				"   PID     USER     %CPU  %MEM   RSS   START    TIME    COMMAND",
				"--------------------------------------------------------------------------------",
				"      1 root         1.5   0.8  30.9M  Jul10  00:28:35  ─┬─ /sbin/launchd",
				"    100 daemon       0.0   0.1  10.0M  Jul10        --   ├─┬─ /usr/sbin/sshd",
				"    200 alice       25.3  15.2   1.0G  --        25hrs   │ └─┬─ sshd: alice [priv]",
				"    201 alice        0.1   0.5   5.0M  13:10  00:02:15   │   └─── -bash",
				"    400 bob         15.7   8.9 200.0M  14:40  00:05:45   └─── node server.js",
			},
		},
		{
			name:         "full username display",
			cli:          CLI{Users: []string{"verylongusername"}},
			maxUserLen:   16,
			showFullUser: true,
			expected: []string{
				"   PID        USER        %CPU  %MEM   RSS   START    TIME    COMMAND",
				"--------------------------------------------------------------------------------",
				"      1 root               1.5   0.8  30.9M  Jul10  00:28:35  ─┬─ /sbin/launchd",
				"    500 verylongusername   0.0   0.1   2.0M  2023         --   └─── /usr/local/bin/custom-daemon",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Create test CLI with showFullUser setting
			testCLI := tt.cli
			testCLI.ShowFullUser = tt.showFullUser
			testCLI.Indent = 2

			// Create a test Proktree instance
			pt := &Proktree{
				processes:   processes,
				children:    pidToChildren,
				skipPids:    skipPids,
				maxStartLen: 5,
				maxTimeLen:  8,
				termWidth:   0,
				cli:         testCLI,
			}

			// Calculate column widths properly
			pt.calculateColumnWidths()

			// Override if test specifies a specific maxUserLen
			if tt.maxUserLen > 0 {
				pt.maxUserLen = tt.maxUserLen
			}

			// Apply filters using the actual filtering logic
			rootPids, pidsToShow := pt.filterProcesses()
			pt.pidsToShow = pidsToShow

			// Should have one root PID
			if len(rootPids) != 1 || rootPids[0] != 1 {
				t.Errorf("Expected root PID 1, got %v", rootPids)
			}

			// Capture output
			var buf strings.Builder
			pt.printHeader(&buf)
			pt.printProcessTree(&buf, 1, true)

			// Get lines from output
			output := strings.TrimRight(buf.String(), "\n")
			var lines []string
			if output != "" {
				lines = strings.Split(output, "\n")
			}

			// Compare output
			if len(lines) != len(tt.expected) {
				t.Errorf("Expected %d lines, got %d", len(tt.expected), len(lines))
				t.Logf("Got:\n%s", strings.Join(lines, "\n"))
				return
			}

			for i, expected := range tt.expected {
				if i >= len(lines) {
					t.Errorf("Missing line %d: expected %q", i, expected)
					continue
				}
				if lines[i] != expected {
					t.Errorf("Line %d mismatch:\ngot:      %q\nexpected: %q", i, lines[i], expected)
				}
			}
		})
	}
}

func TestIndentation(t *testing.T) {
	// Create simple test processes
	processes := map[int]*Process{
		1: {
			PID:       1,
			PPID:      0,
			User:      "root",
			CPUPct:    0.0,
			MemPct:    0.0,
			RSSKB:     1024.0,
			StartTime: nil,
			CPUTime:   0,
			Command:   "init",
		},
		10: {
			PID:       10,
			PPID:      1,
			User:      "user",
			CPUPct:    0.0,
			MemPct:    0.0,
			RSSKB:     1024.0,
			StartTime: nil,
			CPUTime:   0,
			Command:   "parent",
		},
		20: {
			PID:       20,
			PPID:      10,
			User:      "user",
			CPUPct:    0.0,
			MemPct:    0.0,
			RSSKB:     1024.0,
			StartTime: nil,
			CPUTime:   0,
			Command:   "child1",
		},
		21: {
			PID:       21,
			PPID:      10,
			User:      "user",
			CPUPct:    0.0,
			MemPct:    0.0,
			RSSKB:     1024.0,
			StartTime: nil,
			CPUTime:   0,
			Command:   "child2",
		},
		30: {
			PID:       30,
			PPID:      20,
			User:      "user",
			CPUPct:    0.0,
			MemPct:    0.0,
			RSSKB:     1024.0,
			StartTime: nil,
			CPUTime:   0,
			Command:   "grandchild",
		},
	}

	pidToChildren := map[int][]int{
		1:  {10},
		10: {20, 21},
		20: {30},
	}

	skipPids := make(map[int]bool)

	tests := []struct {
		name         string
		indentSize   int
		expectedTree []string
	}{
		{
			name:       "default indentation (2 spaces)",
			indentSize: 2,
			expectedTree: []string{
				"      1 root         0.0   0.0   1.0M  --           --  ─┬─ init",
				"     10 user         0.0   0.0   1.0M  --           --   └─┬─ parent",
				"     20 user         0.0   0.0   1.0M  --           --     ├─┬─ child1",
				"     30 user         0.0   0.0   1.0M  --           --     │ └─── grandchild",
				"     21 user         0.0   0.0   1.0M  --           --     └─── child2",
			},
		},
		{
			name:       "single space indentation",
			indentSize: 1,
			expectedTree: []string{
				"      1 root         0.0   0.0   1.0M  --           --  ─┬ init",
				"     10 user         0.0   0.0   1.0M  --           --   └┬ parent",
				"     20 user         0.0   0.0   1.0M  --           --    ├┬ child1",
				"     30 user         0.0   0.0   1.0M  --           --    │└─ grandchild",
				"     21 user         0.0   0.0   1.0M  --           --    └─ child2",
			},
		},
		{
			name:       "4 space indentation",
			indentSize: 4,
			expectedTree: []string{
				"      1 root         0.0   0.0   1.0M  --           --  ─┬─── init",
				"     10 user         0.0   0.0   1.0M  --           --   └───┬─── parent",
				"     20 user         0.0   0.0   1.0M  --           --       ├───┬─── child1",
				"     30 user         0.0   0.0   1.0M  --           --       │   └─────── grandchild",
				"     21 user         0.0   0.0   1.0M  --           --       └─────── child2",
			},
		},
		{
			name:       "10 space indentation",
			indentSize: 10,
			expectedTree: []string{
				"      1 root         0.0   0.0   1.0M  --           --  ─┬───────── init",
				"     10 user         0.0   0.0   1.0M  --           --   └─────────┬───────── parent",
				"     20 user         0.0   0.0   1.0M  --           --             ├─────────┬───────── child1",
				"     30 user         0.0   0.0   1.0M  --           --             │         └─────────────────── grandchild",
				"     21 user         0.0   0.0   1.0M  --           --             └─────────────────── child2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test Proktree instance with specified indentation
			pt := &Proktree{
				processes:   processes,
				children:    pidToChildren,
				skipPids:    skipPids,
				maxUserLen:  10,
				maxStartLen: 5,
				maxTimeLen:  8,
				termWidth:   0,
				cli:         CLI{Indent: tt.indentSize},
			}

			// No filters - show all
			pt.pidsToShow = nil

			// Capture output
			var buf strings.Builder
			pt.printProcessTree(&buf, 1, true)

			// Get lines from output
			output := strings.TrimRight(buf.String(), "\n")
			var lines []string
			if output != "" {
				lines = strings.Split(output, "\n")
			}

			// Compare output
			if len(lines) != len(tt.expectedTree) {
				t.Errorf("Expected %d lines, got %d", len(tt.expectedTree), len(lines))
				t.Logf("Got:\n%s", strings.Join(lines, "\n"))
				return
			}

			for i, expected := range tt.expectedTree {
				if i >= len(lines) {
					t.Errorf("Missing line %d: expected %q", i, expected)
					continue
				}
				if lines[i] != expected {
					t.Errorf("Line %d mismatch:\ngot:      %q\nexpected: %q", i, lines[i], expected)
				}
			}
		})
	}
}
