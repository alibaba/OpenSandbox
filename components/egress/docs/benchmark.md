# Egress Benchmark

This document describes the **Egress Sidecar** end-to-end benchmark: it compares **dns** and **dns+nft** modes under real conditions for latency and throughput.

## Purpose

- **dns**: DNS proxy only (pass-through), no nftables writes; used as the baseline.
- **dns+nft**: DNS proxy plus synchronous `AddResolvedIPs` before each DNS reply, writing resolved IPs into nftables for
  L2 egress enforcement.

The benchmark runs the same workload in both modes and reports end-to-end latency (P50, P99) and throughput (Req/s) to
measure the overhead of the synchronous nft write path.

## Environment and Flow

- **Environment**: The Egress sidecar runs in a Docker container on the host. The container includes the sidecar (DNS
  proxy and optional nft), iptables redirect of port 53 to the proxy, and the policy server on port 18080. The workload
  runs **inside the same container**: DNS and HTTPS traffic go through the proxy.
- **Flow** (per phase):
    1. Start the sidecar with the chosen mode (`dns` or `dns+nft`).
    2. Wait for health checks, then push the allow list to `/policy` (see domain list below).
    3. Write the domain list into the container as `/tmp/bench-domains.txt` (one `https://<domain>` per line).
    4. **Warm-up**: One request to each of the first 10 domains (10 concurrent), 1 round.
    5. **Timed run**: One request per domain for all domains (N concurrent per round), for 10 rounds; each request
       records `time_namelookup` and `time_total`.
    6. Copy results from the container and compute P50, P99, average latency, and Req/s.
- **Execution order**: **dns+nft** runs first, then **dns**; the comparison table is printed at the end.

## Workload

- **Domain list**: Read from `components/egress/tests/hostname.txt`, one domain per line (lines starting with `#` and
  empty lines are ignored). Default is about 100 resolvable domains.
- **Rounds and concurrency**: The script uses `ROUNDS=10`. Each round issues one HTTPS request per domain in
  `hostname.txt`, with all requests in that round concurrent; 10 rounds total.
- **Total requests**: `TOTAL_REQUESTS = ROUNDS × NUM_DOMAINS` (e.g. 10 × 100 = 1000).
- **Per request**: Inside the container, `curl -o /dev/null -s -w "%{time_namelookup}\t%{time_total}\n"` is used against
  `https://<domain>`, with a 10s timeout per request; the whole benchmark run has a 300s wall-clock timeout.

## Policy

- Policy is default-deny with explicit allow rules: one `{"action":"allow","target":"<domain>"}` per domain in
  `hostname.txt` is sent via `POST /policy`, so every domain used in the benchmark is allowed.

## How to Run

**Script**: `components/egress/tests/bench-e2e-dns-nft.sh`

**Requirements**: Docker and `curl` on the host (for pushing policy); the Egress image includes `curl` for the workload.

**Commands** (from repo root or from `components/egress`):

```bash
./tests/bench-dns-nft.sh
```

The script resolves `tests/hostname.txt` relative to its own path, so the working directory does not need to be changed.

## Configuration

| Item                | Location / variable                    | Default / notes                                |
|---------------------|----------------------------------------|------------------------------------------------|
| Domain list         | `components/egress/tests/hostname.txt` | One domain per line; `#` comments allowed      |
| Rounds              | `ROUNDS` in script                     | 10                                             |
| Per-request timeout | `CURL_TIMEOUT` in script               | 10 seconds                                     |
| Benchmark timeout   | `BENCH_EXEC_TIMEOUT` in script         | 300 seconds (max wall time for the timed run)  |
| Image               | `IMG` in script                        | See script; override for a locally built image |

Changing the number of domains or rounds updates the total request count; the report shows “N rounds × M domains” for
the current config.

## Output and Metrics

- **Terminal**: A table with **Req/s**, **Avg(s)**, **P50(s)**, **P99(s)** for both modes, plus short notes (dns vs
  dns+nft, warm-up, first-resolution cost).
- **Artifacts** (on the host under `/tmp`): `bench-e2e-dns-total.txt`, `bench-e2e-dns+nft-total.txt` (one
  `time_total` per line), and `-namelookup.txt`, `-wall.txt`, etc., for further analysis or plotting.

## Notes

- The first resolution of a domain in dns+nft triggers a DNS lookup and an nft write, so cost is higher; later requests
  for the same domain hit the set and are cheaper. The multi-round, multi-domain design mixes cold and warm resolution.
- In CI (e.g. GitHub Actions), the script wraps the timed-run `docker exec` with `timeout` inside the shell function so
  `timeout` runs a real command, not a function name, avoiding “No such file or directory” errors.
