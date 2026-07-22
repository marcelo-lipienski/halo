# 6. Docker API & Graceful Degradation

## Status
Accepted

## Context
Checking container status requires connecting to the local Docker socket. However, local environments can be run in "offline" mode (e.g. Docker daemon is stopped/unreachable). Failing the entire diagnostic suite when the Docker engine is offline prevents developers from using `halo` to verify local configuration files, environment variable alignment, or local file permissions.

## Decision
1. **Daemon Connection Check:** Detect standard local Docker system socket endpoints (`unix:///var/run/docker.sock` or Windows named pipe).
2. **Graceful Offline Degradation:** If the Docker daemon is offline/unreachable:
   - Downgrade Docker-dependent API validation checks (e.g. container running status) from critical errors to warnings.
   - Continue running independent environment variable validation and local filesystem checks.
3. **Port Collision Exclusions & Single-Port Parsing:**
   - Scan target ports declared in Docker Compose (including short/long-form single port syntax e.g. `80`, `80/tcp`, `80-82`) to ensure host port availability.
   - Bypass port collision alerts if the port is bound by a container belonging to the same project and service (using Docker labels: `com.docker.compose.project` and `com.docker.compose.service`).
4. **Scale Warning:** Warn the user when a single service tries to map more than 64 ports to protect the engine from network timeout delays.

## Consequences
- Allows developers to validate local workspaces and configuration files even when Docker is stopped.
- Avoids false-positive port collision warnings for active services already running in the current directory.
