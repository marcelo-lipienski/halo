# Specification: halo 

## 1. Product Vision & Scope
**halo** is a low-overhead, high-performance CLI tool written in Go ^1.26 designed to instantly diagnose and validate local Docker development environments. It parses standard local configurations (`.env`, `docker-compose.yml`) and actively tests system states (ports, volumes, permissions) to catch configuration drift and onboarding issues before they drain developer hours.

---

## 2. Command Line Interface (CLI) Architecture

The tool must support the following command structure and flags:

### Commands
* `halo check`
  Executes the entire diagnostic suite once and exits.
* `halo version`
  Outputs the current version, Go runtime details, and build commit hash.

### Global Flags
* `--config-dir, -c <path>`
  Path to the directory containing local configuration files (Default: `.`).
* `--env-file, -e <path>`
  Explicit path to the `.env` file (Default: `.env` in config-dir).
* `--compose-file <path>`
  Explicit path(s) to the docker-compose.yml file (can specify multiple times).
* `--fix`
  Automatically attempt to mitigate file permission and missing directory/file issues.
* `--dry-run`
  Preview mitigation steps when running with `--fix` without making modifications on the host filesystem.
* `--quiet, -q`
  Suppresses all standard output while routing critical system/discovery failures to stderr.
* `--format, -f <text|json>`
  Output format for results. `json` format prints a single structured payload to stdout for integration into setup automation scripts (Default: `text`).
* `--verbose, -v`
  Enables debug logging, including raw system error string outputs during validation failures.

---

## 3. Core Engine Lifecycle & Execution Flow

When `halo check` is executed, the engine must execute the following sequential pipeline under a managed lifecycle context (`context.Context`) with a global execution timeout of 10 seconds.

```[1. Discovery] ──> [2. AST/Config Parsing] ──> [3. Concurrent Execution] ──> [4. Output Rendering]```

### Phase 1: Environment Discovery
* Locate `.env` and `docker-compose.yml` (or `docker-compose.yaml`) within the target directory.
* If files are missing, halt execution immediately with `Exit Code 1` and print clear setup instructions.
* Verify that the local Docker daemon is running and accessible via the system socket (`unix:///var/run/docker.sock` or Windows named pipe).

### Phase 2: Configuration Analysis (AST Parsing)
* Extract variable declarations from `.env`.
* Parse `docker-compose.yml` into structural domain types without using heavy, unnecessary system dependencies.
* Resolve dynamic environment parameters using AST-compliant shell-expression parsing (supporting default fallbacks like `${VAR:-default}` and required checks like `${VAR:?error}`).
* Map out defined services, their expected environment mappings, exposed/internal ports, mounted storage volumes, secrets, and configs.

### Phase 3: Concurrent Diagnostic Execution
The suite must run individual check groups concurrently using lightweight, native goroutines with synchronized error/result aggregation. Each check group receives a scoped context timeout of 2 seconds.

#### Check Group A: Environmental Alignment
* **Variables Check:** Ensure every environment variable referenced in `docker-compose.yml` is explicitly defined in the local `.env` file or host environment.
* **Mismatched Types:** Flag variables that are defined but empty if they are structurally required.

#### Check Group B: Network & Port Availability
* **Port Collision Check:** Scan the local host system to ensure target host ports mapped in `docker-compose.yml` are not occupied.
  * *Self-Port Exclusion:* Ports bound by the same container service in the current docker-compose project are bypassed to prevent false-positive collisions.
  * *Scale Protection:* A warning is emitted if a port range maps more than 64 ports to prevent timeouts.
* **Service Reachability:** Verify container running states and health check statuses via the Docker engine API. Treats starting containers or missing containers as non-fatal warnings (`CheckWarning`).

#### Check Group C: Volume & File Permissions
* **Mount Validation:** Identify host paths mounted as volumes.
* **Secrets & Configs Validation:** Identify Docker Secrets and Docker Config files, validating their existence and readability.
* **Permission Verification:** Verify that the current host user has explicit read/write privileges on directories/files.
  * *Auto-Fix & Verification:* When `--fix` is active, missing directories/files are created and permissions are corrected, followed by a re-verification check.

### Phase 4: Output Rendering & Exit Boundaries
* **Text Format:** Print a clean, scannable terminal checklist using standard ANSI codes. Failures must include an indented, explicit mitigation step (e.g., "Run: chmod -R 775 ./storage"). Automatically honors the `NO_COLOR` standard and detects non-TTY outputs.
* **JSON Format:** Stream a single-line minified JSON object containing an overall status string, individual check array results, and execution duration.

---

## 4. Exit Code Matrix

The CLI binary must enforce semantic exit codes to allow clean embedding inside wrapper scripts or local Git hooks:

| Exit Code | Condition | Meaning |
|---|---|---|
| `0` | **Healthy** | All configuration files parsed perfectly and all diagnostic checks passed. |
| `1` | **System Failure** | Core files are missing, Docker daemon is down, or CLI flags are invalid. |
| `2` | **Environment Broken** | Configuration parsed successfully, but one or more critical diagnostic checks failed. |
