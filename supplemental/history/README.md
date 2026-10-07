# Granular history fork deployment

This fork preserves original samples every 15 seconds for 35 days by default. Existing minute maintenance, native rollups and alerts remain available. History settings are read when the hub starts:

```env
BESZEL_HISTORY_INTERVAL_SECONDS=15
BESZEL_HISTORY_RETENTION_DAYS=35
```

Restart/redeploy the hub after changing the environment. Keep its persistent data directory. Increasing retention cannot restore expired samples. Changing the interval only affects new measurements. The History panel uses the effective backend settings; no frontend rebuild is required for configuration changes.

## Native hub inside a Proxmox LXC

The community LXC helper installs the **upstream** binary, and its update command targets upstream releases. It does not automatically select `klausgibin/beszel`. You may reuse the LXC/service it creates, then replace its hub executable with a validated fork build. Use the helper's advanced settings to provision approximately 1–2 vCPUs, 1 GB RAM and 8–10 GB disk for the initial 15-second/35-day setup with up to 20 containers. These are initial allocations to validate with real ingestion.

Sources checked on 2026-10-07:

- https://github.com/community-scripts/ProxmoxVE/blob/main/ct/beszel.sh
- https://github.com/community-scripts/ProxmoxVE/blob/main/install/beszel-install.sh

### Get a fork binary

After each push to `main`, the **Validate and build history hub** GitHub Actions workflow builds the frontend, runs the Go and frontend suites and uploads Linux amd64/arm64 hub binaries and checksums in the `beszel-history-linux` artifact. Only use artifacts from a successful run at the intended commit. A source push alone does not publish a release or replace an installed hub.

Download from the Actions UI or GitHub CLI:

```sh
gh run list --repo klausgibin/beszel --workflow history-fork.yml
gh run download RUN_ID --repo klausgibin/beszel --name beszel-history-linux --dir ./history-hub
cd history-hub
sha256sum -c SHA256SUMS
```

Alternatively build locally from the desired checkout:

```sh
bun install --frozen-lockfile --cwd internal/site
(cd internal/site && bunx lingui compile && bunx tsc -b && bunx vite build)
mkdir -p build/history
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-w -s' -o build/history/beszel-linux-amd64 ./internal/cmd/hub
```

Use `GOARCH=arm64` for an ARM64 node. Build only the hub; current compatible upstream agents can continue to be used. Integration with the agents and cgroup metrics must still be validated in the actual homelab.

### Replace the community helper's hub

Copy the matching binary into the LXC at `/tmp/beszel-history`. On an installation created by the checked helper, the service is `beszel-hub`, binary `/opt/beszel/beszel`, working directory `/opt/beszel`, and default data directory `/opt/beszel/beszel_data`. Verify these paths in `systemctl cat beszel-hub` before applying the following steps.

Stop the hub before taking a complete backup of its data directory (including database journal files). Keep that backup off the LXC if disk space is limited. Retain the old executable for recovery.

```sh
systemctl stop beszel-hub
cp -p /opt/beszel/beszel /opt/beszel/beszel.previous
install -m 0755 /tmp/beszel-history /opt/beszel/beszel.new
mv /opt/beszel/beszel.new /opt/beszel/beszel
mkdir -p /etc/systemd/system/beszel-hub.service.d
cat > /etc/systemd/system/beszel-hub.service.d/history.conf <<'CONFIG'
[Service]
Environment=BESZEL_HISTORY_INTERVAL_SECONDS=15
Environment=BESZEL_HISTORY_RETENTION_DAYS=35
Environment=CHECK_UPDATES=false
CONFIG
systemctl daemon-reload
systemctl start beszel-hub
systemctl status beszel-hub
curl --fail http://127.0.0.1:8090/api/health
```

The fork's `beszel update` command intentionally refuses an upstream replacement. Do not use the community helper's `update` command to update this customized hub; repeat the verified artifact replacement procedure instead. Rollback should restore the matching database backup as well as the executable when a schema change makes that necessary.

Tailscale connectivity must be configured inside or routed into the LXC. An agent in the LXC measures that environment; node, VM and remote Docker metrics still require agents at their respective systems. The hub does not automatically inherit physical-node visibility.

## Container deployment

Build the frontend first, then build the fork's existing hub Dockerfile:

```sh
docker build -f internal/dockerfile_hub -t beszel-history:local .
```

Example compose service:

```yaml
services:
  beszel:
    image: beszel-history:local
    restart: unless-stopped
    ports:
      - "8090:8090"
    environment:
      BESZEL_HISTORY_INTERVAL_SECONDS: "15"
      BESZEL_HISTORY_RETENTION_DAYS: "35"
      CHECK_UPDATES: "false"
    volumes:
      - ./beszel_data:/beszel_data
```

Changing those variables and recreating the service retains the archive in the mounted directory. Do not switch this service back to `henrygd/beszel` or an upstream image expecting the granular History feature to remain.

## Operations

Use the History panel for dates, a system or historical container, original samples, peak-preserving overview charts, daily statistics, coverage, distribution and CSV export. History starts when the fork begins collecting; the upstream live chart's previous second-by-second values were never archived.

Daily percentiles summarize collected samples. CPU represents average activity during a sampling interval, and missed samples remain gaps. Container CPU follows Docker's core accounting and may exceed 100%; container memory uses MiB, whereas host memory/disk charts use percentages. Coverage indicates how much of the requested interval actually has observations.

Monitor database growth, collection duration and memory during historical queries over the first several days. Leave operating space for the SQLite journal, compaction/backup operations and temporary files. SQLite reuses pages freed by retention cleanup; file size does not necessarily shrink immediately when records expire. Configure off-host backups instead of keeping another full database inside a small LXC.
