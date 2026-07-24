# Advanced Usage: halo

This guide covers advanced configuration, CLI flags, merging mechanics, and diagnostic features in **halo**.

---

## Commands

### `halo check`
Runs the full diagnostic suite to validate local system state against declared Docker topologies, environment variables, port availability, container health, and volume permissions.

```bash
halo check
```
*(Executing `halo` with no subcommand defaults to `halo check`)*

### `halo fix`
Automatically mitigates detected configuration, permission, and directory structure issues without requiring manual file system adjustments.

```bash
halo fix
```
*(Equivalent to running `halo check --fix`)*

### `halo init`
Copies or merges missing configuration keys from `.env.example` into `.env`. Automatically flags missing placeholder values that require developer input before execution.

```bash
halo init
```

### `halo doctor`
Inspects host system prerequisites:
* **Docker CLI & Compose v2**: Verifies if Docker Compose v2 is installed.
* **Docker Engine Version**: Inspects running Docker Engine API version.
* **Required CLI Tools**: Checks for `git`, `make`, and `docker` in system `$PATH`.
* **System Memory limits**: Queries total host memory and compares it against memory limit sum declared under `deploy.resources.limits.memory` in services.
* **Free Disk Space**: Queries free space on the current drive and warns if under 2.0 GiB.

```bash
halo doctor
```

### `halo snapshot [file]`
Captures a baseline state snapshot of the local environment (including files, environment variables, ports, and active container states) and writes it to `.halo-snapshot.json` (or a specified output path).

```bash
halo snapshot
# Or save to custom file path:
halo snapshot baseline.json
```

### `halo diff [file]`
Compares current environment state against a previously captured snapshot (`.halo-snapshot.json` or specified file path) and reports any environment drift (modified files, altered environment variables, or port/container changes). Returns exit code 2 if differences are detected.

```bash
halo diff
# Or compare against custom snapshot file:
halo diff baseline.json
```

### `halo version`
Prints the binary version, build commit SHA, and Go runtime details.

```bash
halo version
```

---

## Flags & Advanced Options

### Multi-File Precedence (`--compose-file`)
Specifies one or more explicit `docker-compose` files to load. This flag can be repeated to load multiple files. When this flag is provided, automatic file discovery (including `docker-compose.override.*`) is **disabled**.

```bash
halo check \
  --compose-file docker-compose.yml \
  --compose-file docker-compose.override.yml
```

#### Merge Rules
When multiple files are provided, `halo` merges them **left-to-right**, where each subsequent file overrides or extends the previous one.

| Field | Behaviour |
|---|---|
| `image`, `container_name`, `entrypoint`, `command` | **Override** — latter value replaces former if non-empty |
| `environment` | **Merge** — latter keys override former keys; new keys are appended |
| `ports` | **Append** — all port entries from all files are combined |
| `volumes` | **Override by target path** — if a latter file defines a volume with the same container target path, it replaces the former entry. |
| `secrets`, `configs`, root-level `volumes` | **Override** — latter definition replaces former by name |

#### Merge Example
Given these two files:

```yaml
# docker-compose.yml
services:
  app:
    image: myapp:latest
    environment:
      - DB_HOST=localhost
    ports:
      - "8080:80"
    volumes:
      - ./data:/app/data
```

```yaml
# docker-compose.override.yml
services:
  app:
    image: myapp:dev
    environment:
      - DB_HOST=db
      - DEBUG=true
    ports:
      - "9229:9229"
    volumes:
      - ./data-dev:/app/data
```

After merging, the effective configuration is:

```yaml
services:
  app:
    image: myapp:dev           # overridden
    environment:
      DB_HOST: db              # overridden
      DEBUG: "true"            # appended
    ports:
      - "8080:80"              # kept
      - "9229:9229"            # appended
    volumes:
      - ./data-dev:/app/data   # overrode /app/data mount
```

---

### Configuration & Environment Paths

* **`--config-dir, -c <path>`**: Sets the root directory where `halo` auto-discovers configuration files (`.env`, `docker-compose.yml`, `docker-compose.override.yml`). Defaults to `.`.
* **`--env-file, -e <path>`**: Specifies an explicit path to the `.env` file, overriding the auto-discovery path.

---

### Runtime Controls

* **`--fix`**: Automatically creates missing directories and repairs read/write permissions on host-mounted volume paths.
* **`--interactive, -i`**: Requires user confirmation before applying mitigations to the host system.
* **`--dry-run, -d`**: Previews mitigations or state snapshot operations without modifying the filesystem.
* **`--quiet, -q`**: Suppresses standard output.
* **`--verbose, -v`**: Enables debug logs, including raw system error reports.
* **`--watch, -w`**: Live-monitors configuration files. Clears the terminal and instantly re-runs diagnostics upon any update or deletion.

---

### Output Formatting (`--format, -f`)
Controls format of diagnostic reports. Accepts `text` (default) or `json`.

* **Text format** (default) — human-readable, ANSI-coloured checklist. Colors automatically suppress when piped or when `NO_COLOR` is set.
* **JSON format** — single-line minified JSON payload to `stdout`, ideal for scripts or Git hooks:
  ```json
  {"status":"healthy","duration_ms":38,"checks":[{"group":"Environmental Alignment","name":"Variables Check","status":"passed"}]}
  ```

---

## Auto-Discovery Precedence
When no flags are specified, `halo` resolves configuration files from `--config-dir` in this order:
1. **Primary compose file**: Prefers `docker-compose.yml` over `docker-compose.yaml`.
2. **Override file**: Automatically merges `docker-compose.override.yml` (or `.yaml`) on top of the primary if it exists.
3. **Env file**: Loaded from `<config-dir>/.env`.

---

## Known Limitations

### Port Bypass Self-Exclusion
When checking port collisions, `halo` excludes ports bound by the service's own active containers using standard Docker Compose metadata labels (`com.docker.compose.project` and `com.docker.compose.service`). Standalone containers run without these labels (e.g. via raw `docker run`) will always be flagged as port collisions.
