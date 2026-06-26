# my-replica

Interactive TUI tool for provisioning **MySQL Master/Replica** replication with Docker — written in Go.

The original repository documented manual shell steps. **my-replica** automates the full workflow: Docker networking, container lifecycle, config generation, dump/import, replication setup, status checks, and safe load testing.

## Why this project

The old manual README had several issues that this tool fixes:

| Old manual step | my-replica behavior |
|---|---|
| `mysqldump ... < data.sql` for import | Uses **`mysql`** client for import |
| `--link` between containers | Uses a dedicated **Docker network** |
| Hard-coded `mysql:latest` | Configurable image tag (default **`8.0`**) |
| Legacy `CHANGE MASTER TO` only | Modern **`CHANGE REPLICATION SOURCE TO`** with legacy fallback |
| Manual dump copy paths | Handles dump via container volume + host-stream fallback |

## Features

- Single binary for **Linux** and **Windows**
- k9s-style TUI ([tview](https://github.com/rivo/tview))
- Interactive setup form with sensible defaults
- Docker daemon validation, image pull, network, volumes, ports
- Full replication bootstrap (user, dump, import, START REPLICA)
- Dump strategy with **3 fallbacks**
- Replication status (IO/SQL threads, lag, log position, errors)
- Safe load test using dedicated `replication_bench` table only
- Down / cleanup / reset operations

## Prerequisites

- **Go 1.26+** (tested with Go 1.26.4)
- **Docker** installed and running
- Docker daemon accessible from your user session
- Free host ports for master and slave (defaults: `33060`, `33061`)

### Install Docker

- Linux: [https://docs.docker.com/engine/install/](https://docs.docker.com/engine/install/)
- Windows: [Docker Desktop](https://docs.docker.com/desktop/setup/install/windows-install/)

Verify:

```bash
docker info
docker ps
```

## Build

```bash
make tidy
make build
```

Cross-compile:

```bash
make build-linux
make build-windows
```

Manual build examples:

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o dist/my-replica-linux-amd64 ./cmd/my-replica

# Windows amd64
GOOS=windows GOARCH=amd64 go build -o dist/my-replica-windows-amd64.exe ./cmd/my-replica
```

Binary output:

- `bin/my-replica` (local build)
- `dist/my-replica-linux-amd64`
- `dist/my-replica-windows-amd64.exe`

## Run

```bash
make run
# or
./bin/my-replica
```

## TUI shortcuts

| Key | Action |
|-----|--------|
| `s` | Interactive setup |
| `t` | Show replication status |
| `l` | Safe load test |
| `d` | Down (stop & remove containers) |
| `r` | Reset (down + wipe data + setup again) |
| `c` | Cleanup (down + remove network + dump files) |
| `q` | Quit |

## Setup flow

When you press **`s`**, the tool collects:

- MySQL image version (e.g. `8.0`, `latest`)
- Database name, root password
- Replication user/password
- Master/slave host ports
- Docker network name
- Work directory
- Master/slave container names

Then it automatically:

1. Validates Docker daemon
2. Creates work directories and renders `master.cnf` / `slave.cnf`
3. Creates Docker network
4. Starts master with binary logging enabled
5. Creates replication user on master
6. Captures binlog position **before** dump
7. Dumps database (with fallback strategies)
8. Starts slave and imports dump with **`mysql`**
9. Configures replication (`CHANGE REPLICATION SOURCE TO`, legacy fallback)
10. Verifies replica status

Default values are suitable for local development.

## Status

Press **`t`** to display:

- Replica IO / SQL running state
- Seconds behind source
- Current source log file and position
- Replication errors (if any)

## Safe load test

Press **`l`** to run a non-destructive benchmark:

- Creates/uses table **`replication_bench`** only
- Writes to **master** only
- Verifies row counts on slave
- Reports throughput (rows/sec) and replica lag
- Optional cleanup of benchmark table

**Warning:** Even though the test is scoped to a dedicated table, it can still increase CPU, disk I/O, binlog size, and replica lag. **Do not run on production.**

## Down / cleanup / reset

| Command | Behavior |
|---------|----------|
| **Down (`d`)** | Stop and remove master/slave containers |
| **Cleanup (`c`)** | Down + remove Docker network + delete dump files (data dirs kept) |
| **Reset (`r`)** | Down + delete data directories + full setup again (confirmation required) |

## Project layout

```
my-replica/
├── cmd/my-replica/main.go
├── internal/
│   ├── app/
│   ├── docker_mgr/
│   ├── sql_mgr/
│   ├── tester/
│   ├── ui/
│   └── utils/
├── templates/
│   ├── master.cnf.tmpl
│   └── slave.cnf.tmpl
├── Makefile
└── README.md
```

## Troubleshooting

### Docker daemon not available

```
docker unavailable: docker daemon ping failed
```

- Start Docker Desktop (Windows) or Docker service (Linux)
- Ensure your user can access `/var/run/docker.sock` (Linux)

### Port already in use

Change master/slave ports in the setup form, or stop the conflicting service:

```bash
docker ps
netstat -ano | findstr 33060   # Windows
ss -lntp | grep 33060          # Linux
```

### Replica IO/SQL not running

Press **`t`** and check `Last IO Err` / `Last SQL Err`.

Common causes:

- Wrong replication credentials
- Master not reachable on Docker network
- Binlog position mismatch (reset and run setup again)

### Dump failures

The tool tries three strategies:

1. Standard `mysqldump`
2. `mysqldump --single-transaction --set-gtid-purged=OFF`
3. Host-stream via `docker exec`

If all fail, check master logs:

```bash
docker logs <master-container>
```

## Security notes

- Passwords are **not** written to activity logs (commands are sanitized)
- Default passwords are for **local development only**
- Change all credentials before any shared or production-like environment
- Bind ports to localhost-only if you expose Docker on a shared machine

## Development

```bash
make fmt
make tidy
make test
make build
```

## License

See repository license. Manual steps from the original README are preserved in `Manual-running.md` for reference.
