package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Process represents a system process with platform-neutral data
type Process struct {
	PID       int
	PPID      int
	User      string
	CPUPct    float64
	MemPct    float64
	RSSKB     float64
	StartTime *time.Time // nil if unknown
	CPUTime   time.Duration
	Command   string
}

// Platform-specific operations
type Platform interface {
	GetProcesses() ([]Process, error)
}

// GetPlatform returns the appropriate platform implementation
func GetPlatform() Platform {
	switch runtime.GOOS {
	case "darwin":
		return &Darwin{}
	case "linux":
		return &Linux{}
	default:
		panic(fmt.Sprintf("unsupported platform: %s", runtime.GOOS))
	}
}

// Darwin implements process operations for macOS
type Darwin struct{}

func (d *Darwin) GetProcesses() ([]Process, error) {
	// Get process info including PPID with macOS-specific lstart
	cmd := exec.Command("ps", "-axo", "pid,ppid,user,pcpu,pmem,rss,lstart,time,command")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run ps: %v", err)
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
		user := fields[2]
		cpuPct, _ := strconv.ParseFloat(fields[3], 64)
		memPct, _ := strconv.ParseFloat(fields[4], 64)
		rssKb, _ := strconv.ParseFloat(fields[5], 64)

		// Parse lstart and time
		// lstart format: "Thu Jul 10 15:37:36 2025" (6 fields)
		// After that comes the TIME field, then COMMAND
		var startRaw string
		var timeStr string
		var cmd string

		// Find where TIME field starts (after year in lstart)
		// lstart is 5 fields starting at field 6 (Thu Jul 10 15:37:36 2025)
		if len(fields) >= 12 {
			// Standard format: fields 6-10 are lstart (Thu Jul 10 15:37:36 2025)
			// field 11 is TIME
			// field 12+ is COMMAND
			startRaw = strings.Join(fields[6:11], " ") // Include the year
			timeStr = fields[11]
			cmd = strings.Join(fields[12:], " ")
		} else {
			// Fallback for unexpected format
			startRaw = ""
			timeStr = "--"
			cmd = strings.Join(fields[8:], " ")
		}

		// Parse start time
		var startTime *time.Time
		if startRaw != "" {
			if t, err := parseDarwinStartTime(startRaw); err == nil {
				startTime = &t
			}
		}

		// Parse CPU time (macOS format: MM:SS.ss)
		cpuTime := parseMacOSCPUTime(timeStr)

		processes = append(processes, Process{
			PID:       pid,
			PPID:      ppid,
			User:      user,
			CPUPct:    cpuPct,
			MemPct:    memPct,
			RSSKB:     rssKb,
			StartTime: startTime,
			CPUTime:   cpuTime,
			Command:   cmd,
		})
	}

	return processes, nil
}

// Linux implements process operations for Linux
type Linux struct{}

func (l *Linux) GetProcesses() ([]Process, error) {
	// Use Linux ps with -D flag to specify exact lstart format
	// This gives us an ISO-like timestamp that's easy to parse
	cmd := exec.Command("ps", "-D", "%Y-%m-%d %H:%M:%S", "-eo", "pid,ppid,user,pcpu,pmem,rss,lstart,time,cmd")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run ps: %v", err)
	}

	var processes []Process
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Scan() // Skip header

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		if len(fields) < 10 {
			continue
		}

		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		user := fields[2]
		cpuPct, _ := strconv.ParseFloat(fields[3], 64)
		memPct, _ := strconv.ParseFloat(fields[4], 64)
		rssKb, _ := strconv.ParseFloat(fields[5], 64)

		// Parse start time from ISO-like format
		// With -D "%Y-%m-%d %H:%M:%S", lstart is 2 fields
		// fields[6] = date (YYYY-MM-DD)
		// fields[7] = time (HH:MM:SS)
		var startTime *time.Time
		startStr := fields[6] + " " + fields[7]
		if t, err := time.Parse("2006-01-02 15:04:05", startStr); err == nil {
			startTime = &t
		}

		// Parse CPU time (Linux format: [DD-]HH:MM:SS)
		// fields[8] = TIME
		timeStr := fields[8]
		cpuTime := parseLinuxCPUTime(timeStr)

		// Parse command
		// fields[9+] = COMMAND
		cmd := strings.Join(fields[9:], " ")

		processes = append(processes, Process{
			PID:       pid,
			PPID:      ppid,
			User:      user,
			CPUPct:    cpuPct,
			MemPct:    memPct,
			RSSKB:     rssKb,
			StartTime: startTime,
			CPUTime:   cpuTime,
			Command:   cmd,
		})
	}

	return processes, nil
}

// parseDarwinStartTime parses macOS lstart format
func parseDarwinStartTime(startRaw string) (time.Time, error) {
	// Format from ps: "Thu Jul 10 15:37:36 2025"
	formats := []string{
		"Mon Jan _2 15:04:05 2006", // Single digit day with padding
		"Mon Jan  2 15:04:05 2006", // Double space before single digit
		"Mon Jan 2 15:04:05 2006",  // Single space
		"Mon Jan 02 15:04:05 2006", // Zero-padded day
	}

	for _, format := range formats {
		if t, err := time.Parse(format, startRaw); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse start time: %s", startRaw)
}

// parseMacOSCPUTime parses macOS CPU time format: MM:SS.ss
func parseMacOSCPUTime(timeStr string) time.Duration {
	if timeStr == "" {
		return 0
	}

	// macOS format is always MM:SS.ss (minutes:seconds with fraction)
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0
	}

	mins, _ := strconv.Atoi(parts[0])
	secs, _ := strconv.ParseFloat(parts[1], 64)
	return time.Duration(mins)*time.Minute + time.Duration(secs*float64(time.Second))
}

// parseLinuxCPUTime parses Linux CPU time format: [DD-]HH:MM:SS
func parseLinuxCPUTime(timeStr string) time.Duration {
	if timeStr == "" || timeStr == "00:00:00" {
		return 0
	}

	// Check for days: DD-HH:MM:SS
	var days int
	if idx := strings.Index(timeStr, "-"); idx > 0 {
		days, _ = strconv.Atoi(timeStr[:idx])
		timeStr = timeStr[idx+1:]
	}

	// Linux format is HH:MM:SS
	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	mins, _ := strconv.Atoi(parts[1])
	secs, _ := strconv.Atoi(parts[2])
	
	totalHours := days*24 + hours
	return time.Duration(totalHours)*time.Hour + time.Duration(mins)*time.Minute + time.Duration(secs)*time.Second
}
