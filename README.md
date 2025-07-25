# proktree

Print your processes as a nice tree. Filter by process id, user, and command substring.

Works on macOS and Linux.

macOS, particularly, lacks a native tree display, like Linux's `ps f`.

## Sample output

```
# proktree -s runsvdir

   PID     USER    %CPU  %MEM    RSS     START     TIME    COMMAND
--------------------------------------------------------------------------------
      1 root        0.1   0.0  160.5M    Jul10   10:45.64  ─┬─ /sbin/launchd
  99856 jeremyw     0.0   0.0    1.7M    00:58    0:00.78   └─┬─ /usr/local/bin/runsvdir
  54288 jeremyw     0.0   0.0    1.4M    12:51    0:00.01     └─┬─ runsv nginx
  54289 jeremyw     0.0   0.0    1.8M    12:51    0:00.02       ├─── svlogd -tt .
  54290 jeremyw     0.0   0.0  138.7M    12:51    0:17.00       └─── nginx: master process
```

## Installation

### Via Homebrew
```bash
brew tap jeremywohl/tap
brew install proktree
```

### Via Go
```bash
go install github.com/jeremywohl/proktree@latest
```

## Usage

```bash
# Show all processes
proktree

# Show help
proktree -h

# Filter by current user
proktree --me
# or
proktree --mine

# Filter by specific user
proktree -u postgres

# Filter by process ID (show process and its entire tree)
proktree -p 1234

# Filter by command string (case-sensitive)
proktree -s nginx

# Filter by command string (case-insensitive)
proktree -i NGINX

# Combine multiple filters (OR logic)
proktree -p 1234 -u www-data -s apache

# Show full usernames and commands
proktree --long-users --long-commands

# Customize tree indentation (e.g., 4 spaces)
proktree --indent 4
```

## Command-Line Options

| Option | Long Form | Description |
|--------|-----------|-------------|
| `-p` | `--pid` | Show only parents and descendants of process PID (can be specified multiple times) |
| `-u` | `--user` | Show only parents and descendants of processes of USER (can be specified multiple times) |
| | `--me` | Show only parents and descendants of processes of current user |
| | `--mine` | Show only parents and descendants of processes of current user (alias for --me) |
| `-s` | `--string` | Show only parents and descendants of process names containing STRING (can be specified multiple times) |
| `-i` | `--string-insensitive` | Show only parents and descendants of process names containing STRING case-insensitively (can be specified multiple times) |
| | `--long-users` | Show full usernames, without truncation |
| | `--long-commands` | Show full commands, without truncation |
| | `--indent` | Set the number of spaces for each indentation level (default: 2) |
| `-v` | `--version` | Show version and exit |
| `-h` | `--help` | Show help message |

### Column Descriptions

- **PID**: Process ID
- **USER**: Process owner (truncated to 10 chars unless `--long-users` is used)
- **%CPU**: CPU usage percentage
- **%MEM**: Memory usage percentage
- **RSS**: Resident Set Size (memory in MB/GB)
- **START**: Process start time
  - HH:MM for processes started within 24 hours
  - MonDD for processes started this calendar year
  - YYYY for processes started in previous calendar years
- **TIME**: Cumulative CPU time
  - -- for zero CPU time
  - HH:MM:SS for under 24 hours
  - XYhrs for 24+ hours (right-justified)
- **COMMAND**: Process tree visualization and command line

## Examples

### Find all database processes
```bash
proktree -s postgres -s mysql -s mongodb
```

### Show current user's processes
```bash
proktree --me
```

### Show a specific service and its supervision tree
```bash
proktree -s runsvdir
```

### Find all processes owned by web server users
```bash
proktree -u www-data -u nginx -u apache
```

### Debug a specific process and its entire process tree
```bash
proktree -p 12345
```

### Find all Node.js processes (case-insensitive)
```bash
proktree -i node
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
