<p align="center">
  <img src="monkey.png" alt="Chaos Monkey" width="150">
</p>

# Chaos Monkey

A Kubernetes chaos tool that kills pods based on configurable profiles. Supports eviction (PDB-aware), delete, and force-delete modes.

## Installation

```bash
helm upgrade --install chaosmonkey oci://ghcr.io/postfinance/charts/chaosmonkey \
  --namespace kube-chaosmonkey --create-namespace \
  --set-json 'profiles={"default":{"minAge":"1h","maxAge":"14d"}}'
```

## How it works

Chaos Monkey does not naively kill random pods. Instead it pre-calculates a random, detereministic kill time for each pod and kills them when the time comes. The time when pods are allowed to be killed and method of killing is highly configurable through profiles. This ensures:

* Pods have a maximum lifespan: This can be useful when pods have to be recreated across a cluster or if you want to drain nodes gracefully over time
* No change, when multiple replicas of chaos monkey run, for example during an upgrade: Both calculate the same kill time.

## Profiles

Profiles describe when and how pods are killed. There can be a default profile and pods can also self-select a profile by specifying a label: `postfinance.ch/chaos-monkey-profile=<name>`.

| Field | Description |
| --- | --- |
| `minAge` | Minimum pod age before it becomes eligible for a kill. |
| `maxAge` | Maximum pod age — kill time falls within `[minAge, maxAge]` |
| `killMode` | How to kill: `evict` (default, PDB-aware), `delete`, or `force-delete` |
| `excludedTimes` | Time ranges (HH:MM-HH:MM) during which no kills may happen (may cross midnight like 17:00-08:00) |
| `excludedDays` | Days of the week to skip. (`sat`, `sun`) |
| `excludedDates` | Specific dates to skip (`YYYY-MM-DD`) |

Durations support the usual golang syntax (`s`, `m`, `h`) and additionally `d` (days = 24h), and `y` (years = 8760h) suffixes.

Example:

```yaml
default:
  minAge: 1h
  maxAge: 14d
  killMode: evict
  excludedTimes:
    - "17:00-08:00"
    - "12:00-13:00"
  excludedDays:
    - sat
    - sun
  excludedDates:
    - "2026-12-31"

aggressive:
  minAge: 10m
  maxAge: 1h
  killMode: delete
```

### Exclusions

You can exclude namespaces from chaos-monkey with a `postfinance.ch/chaos-monkey-exclusion=true` label.

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--profiles` | | Path to profiles YAML file (required) |
| `--defaultProfile` | `default` | Name of profile to use for pods without a profile label |
| `--interval` | `1m` | Calc loop interval |
| `--listen` | `:8080` | HTTP listen address |
| `--log-level` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `--dry-run` | `false` | Log kills without performing them. Good to test potential impact of a profile. |
| `--dashboard` | `true` | Enable web dashboard |
| `--dms-enabled` | `false` | Enable dead man's switch (see below for explanation) |
| `--dms-auto-resume` | `false` | Auto-resume when lease is renewed after expiry |

## Subcommands

### `renew-lease`

Renews the dead man's switch Lease object. Used by the canary CronJob.

```bash
chaosmonkey renew-lease [--lease-duration SECONDS]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--lease-duration` | `120` | Lease duration in seconds |

## Dead Man's Switch

The dead man's switch (DMS) is a safety mechanism that automatically suspends evictions when the cluster has scheduling or image-pull problems. During such times, it's probably best to keep pods running. This feature is disabled by default.

1. A **CronJob** (default: every minute) runs the same chaosmonkey image with `imagePullPolicy: Always` and executes the `renew-lease` subcommand. This renews a Kubernetes [Lease](https://kubernetes.io/docs/concepts/architecture/leases/) object.
2. The chaosmonkey watches the Lease. If the Lease has expired (`renewTime + leaseDurationSeconds`), the DMS triggers and **suspends all evictions**.
3. Optionally (disabled by default), evictions **auto-resume** once the Lease is renewed again.

Because the CronJob uses `imagePullPolicy: Always`, a failure to pull the image (e.g. registry down, node scheduling issues) will prevent the lease renewal, triggering the switch.

### Configuration

In helm values:

```yaml
deadManSwitch:
  enabled: true
  schedule: "* * * * *"     # CronJob schedule for lease renewal
  leaseDuration: 120        # Lease validity in seconds; DMS triggers after expiry
  autoResume: false         # Auto-resume killing after lease comes back
```

### Events

When evictions are suspended, a `Warning` event with reason `EvictionsSuspended` is emitted. When evictions are resumed, an `EvictionsResumed` event is emitted. These events fire for all suspend/resume actions (DMS, manual, dashboard).

## Suspend / Resume

Evictions can be suspended at runtime without restarting:

```bash
# Suspend
curl -X POST http://chaosmonkey:8080/suspend

# Resume
curl -X POST http://chaosmonkey:8080/resume
```

While suspended:

* The calc loop continues (schedule is maintained).
* The kill loop is paused — no pods are killed.
* The dashboard state indicator turns red and shows "Suspended".

## Endpoints

| Path | Method | Description |
|---|---|---|
| `/` | GET | Live dashboard with stats, profiles, recent kills, upcoming schedule |
| `/metrics` | GET | Prometheus metrics |
| `/healthz` | GET | Health check |
| `/suspend` | POST | Suspend evictions |
| `/resume` | POST | Resume evictions |

## Metrics

Only application metrics are listed here (prefix `chaosmonkey_`). Go/runtime/process metrics are intentionally omitted.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `chaosmonkey_calc_duration_seconds` | Histogram | none | Duration of each calc tick. |
| `chaosmonkey_dms_expired` | Gauge | none | Dead man's switch state: `1` when lease is expired, else `0`. |
| `chaosmonkey_info` | Gauge | `dry_run`, `timezone` | Build/runtime info metric with constant value `1`. |
| `chaosmonkey_kill_errors_total` | Counter | `reason` | Kill failures. Known reasons: `pdb_blocked`, `error`. |
| `chaosmonkey_pods_evaluated_total` | Counter | none | Pods evaluated during calc loop. |
| `chaosmonkey_pods_excluded_total` | Counter | none | Pods skipped due to namespace exclusion label. |
| `chaosmonkey_pods_killed_total` | Counter | `profile`, `mode`, `dry_run` | Pods selected for kill action (includes dry-run actions). |
| `chaosmonkey_resumptions_total` | Counter | `reason` | Resume actions by source. Common reasons: `dms`, `dashboard`, `manual`. |
| `chaosmonkey_suspended` | Gauge | none | Suspension state: `1` when evictions are suspended, else `0`. |
| `chaosmonkey_suspensions_total` | Counter | `reason` | Suspend actions by source. Common reasons: `dms`, `dashboard`, `manual`. |
| `chaosmonkey_upcoming_kills` | Gauge | none | Count of scheduled kills in the next 24h. |
