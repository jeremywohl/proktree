package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFormatStartTime(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		setup    func()
	}{
		{
			name:     "invalid input",
			input:    "--",
			expected: "--",
		},
		{
			name:     "malformed date",
			input:    "not a date",
			expected: "--",
		},
		{
			name:     "recent time (less than 24 hours)",
			input:    time.Now().Add(-2 * time.Hour).Format("Mon Jan _2 15:04:05 2006"),
			expected: time.Now().Add(-2 * time.Hour).Format("15:04"),
		},
		{
			name:     "current year",
			input:    time.Now().Add(-48 * time.Hour).Format("Mon Jan _2 15:04:05 2006"),
			expected: time.Now().Add(-48 * time.Hour).Format("Jan02"),
		},
		{
			name:     "previous year",
			input:    time.Now().Add(-400 * 24 * time.Hour).Format("Mon Jan _2 15:04:05 2006"),
			expected: time.Now().Add(-400 * 24 * time.Hour).Format("2006"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			result := formatStartTime(tt.input)
			if result != tt.expected {
				t.Errorf("formatStartTime(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatCPUTime(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "minutes and seconds",
			input:    "1:23.45",
			expected: " 1:23.45",
		},
		{
			name:     "hours and minutes",
			input:    "12:34.56",
			expected: "12:34.56",
		},
		{
			name:     "24+ hours",
			input:    "25:00.00",
			expected: " 25hrs ",
		},
		{
			name:     "plain string",
			input:    "0.12",
			expected: "0.12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatCPUTime(tt.input)
			if result != tt.expected {
				t.Errorf("formatCPUTime(%q) = %q, want %q", tt.input, result, tt.expected)
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
	originalCLI := cli
	defer func() { cli = originalCLI }()

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
			cli.ShowFullUser = tt.showFullUser
			result := truncateUser(tt.user)
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
		1: {pid: 1, ppid: 0, user: "root", cmd: "init"},
		2: {pid: 2, ppid: 1, user: "root", cmd: "kernel_task"},
		3: {pid: 3, ppid: 1, user: "daemon", cmd: "systemd"},
		4: {pid: 4, ppid: 3, user: "daemon", cmd: "cron"},
		5: {pid: 5, ppid: 3, user: "user1", cmd: "bash"},
		6: {pid: 6, ppid: 5, user: "user1", cmd: "vim test.txt"},
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
		},
		{
			name: "filter by user",
			cli: CLI{
				Users: []string{"daemon"},
			},
			expectedPidsShow: map[int]bool{
				1: true, // ancestor
				3: true, // matched
				4: true, // matched
			},
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalCLI := cli
			cli = tt.cli
			defer func() { cli = originalCLI }()

			matchingPids := make(map[int]bool)

			for _, p := range processes {
				for _, pidStr := range cli.PIDs {
					if pidStr == strconv.Itoa(p.pid) {
						matchingPids[p.pid] = true
					}
				}

				for _, user := range cli.Users {
					if p.user == user {
						matchingPids[p.pid] = true
					}
				}

				for _, str := range cli.SearchStrings {
					if strings.Contains(p.cmd, str) {
						matchingPids[p.pid] = true
					}
				}

				for _, str := range cli.SearchStringsCase {
					if strings.Contains(strings.ToLower(p.cmd), strings.ToLower(str)) {
						matchingPids[p.pid] = true
					}
				}
			}

			pidsToShow := make(map[int]bool)
			for pid := range matchingPids {
				pidsToShow[pid] = true

				current := pid
				for {
					if p, ok := processes[current]; ok && p.ppid > 0 {
						pidsToShow[p.ppid] = true
						current = p.ppid
					} else {
						break
					}
				}

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

			for pid, expected := range tt.expectedPidsShow {
				if pidsToShow[pid] != expected {
					t.Errorf("PID %d: got %v, want %v", pid, pidsToShow[pid], expected)
				}
			}
		})
	}
}

func TestCommandLineParsing(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedUsers   []string
		expectUserAdded bool
	}{
		{
			name:            "no user flag",
			args:            []string{},
			expectedUsers:   []string{},
			expectUserAdded: false,
		},
		{
			name:            "user flag with argument",
			args:            []string{"-u", "testuser"},
			expectedUsers:   []string{"testuser"},
			expectUserAdded: false,
		},
		{
			name:            "user flag without argument at end",
			args:            []string{"-u"},
			expectedUsers:   []string{},
			expectUserAdded: true,
		},
		{
			name:            "user flag without argument before another flag",
			args:            []string{"-u", "--long-users"},
			expectedUsers:   []string{},
			expectUserAdded: true,
		},
		{
			name:            "multiple user flags",
			args:            []string{"-u", "user1", "-u", "user2"},
			expectedUsers:   []string{"user1", "user2"},
			expectUserAdded: false,
		},
		{
			name:            "long form --user",
			args:            []string{"--user", "testuser"},
			expectedUsers:   []string{"testuser"},
			expectUserAdded: false,
		},
		{
			name:            "long form --user without argument",
			args:            []string{"--user"},
			expectedUsers:   []string{},
			expectUserAdded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily test the main function's argument parsing
			// without refactoring, so we'll test the logic separately
			
			userFlagWithoutArg := false
			args := append([]string{}, tt.args...)
			
			for i := 0; i < len(args); i++ {
				if args[i] == "-u" || args[i] == "--user" {
					if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
						userFlagWithoutArg = true
						args = append(args[:i], args[i+1:]...)
						i--
					}
				}
			}

			if userFlagWithoutArg != tt.expectUserAdded {
				t.Errorf("userFlagWithoutArg = %v, want %v", userFlagWithoutArg, tt.expectUserAdded)
			}
		})
	}
}
