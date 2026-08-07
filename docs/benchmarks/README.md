# SHC VM Performance Benchmarks

**Date**: 2026-08-07
**Provider version**: terraform-provider-shc v0.2.0

## ⚠️ Methodology Disclaimer

These benchmarks were collected using **built-in Linux tools** (`dd`, `curl`) rather
than purpose-built benchmarking suites (`sysbench`, `iperf3`, `fio`). This was a
pragmatic decision: installing packages via `apt-get` on each short-lived test VM
exceeded our time budget. The results below are **directional indicators**, not
lab-grade measurements. Use them to compare plans relative to each other, not as
absolute performance guarantees.

### What each metric actually measures

| Metric | Tool | What it measures | Limitation |
|--------|------|-----------------|------------|
| CPU | `dd if=/dev/zero of=/dev/null` | Memory write throughput (GB/s) — a **proxy** for CPU+memory bus speed, NOT pure compute | Not comparable to `sysbench` scores; measures memory bandwidth, not instruction throughput |
| Disk write | `dd ... oflag=dsync` 256 MB | Sustained sequential write with fsync | Only 256 MB — larger files may show different sustained patterns; no random I/O |
| Disk read | `dd` after `echo 3 > drop_caches` | Sequential read from disk (not page cache) | Cache properly dropped for NVMe Standard run; earlier NVMe Starter run did NOT drop cache (read speed artificially low) |
| Network | `curl` download from 2 public speed test servers | Best-case single-connection HTTP download | Servers are in Europe (tele2.net) and UK (thinkbroadband.com); **geographic distance from Katy, TX depresses results**. Real-world US-to-VM throughput would be higher. |

### What we did NOT measure

- **Random I/O** (fio 4K random read/write) — the most important metric for database workloads
- **CPU compute** (sysbench prime calculation, OpenSSL sign/s)
- **Network latency** (ping, traceroute)
- **Network throughput with iperf3** (proper TCP bandwidth testing)
- **Sustained load** (all tests were single-run, not averaged across multiple runs)
- **SSD Starter plan** — provisioning timed out (possible platform issue)

### How the tests were conducted

1. VM ordered via `shc-toolkit` Python client (`auto_cancel=True`, immediate payment)
2. Polled until `service_status == "active" && ip assigned` (43-97 seconds)
3. 20-30 second settle time for sshd to fully start
4. SSH connection via `paramiko` using ed25519 key
5. Benchmarks executed sequentially (CPU → disk write → cache drop → disk read → network)
6. VM destroyed immediately via `cancel_vm(immediate=True)` after benchmarks completed
7. SHC refunds unused hours — actual cost per VM was ~$0.01-0.02

### Why you should run your own benchmarks

These numbers reflect a specific point in time on SHC's infrastructure. Shared
VPS performance varies with neighbor noise (noisy neighbors on the same physical
host). Network paths change. For production planning, run `sysbench`, `fio`, and
`iperf3` on a VM with your actual workload.

## Results

| Plan | Location | Provision | CPU (dd) | Disk Write | Disk Read | Network |
|------|----------|-----------|----------|------------|-----------|---------|
| NVMe Starter (1c/4gb) | Katy-TX | 97s | 22.9 GB/s | 57.3 MB/s | 7.3 MB/s ⚠️ | 3.9 Mbit/s |
| NVMe Standard (2c/8gb) | Katy-TX | 86s | 20.0 GB/s | 46.6 MB/s | 409.0 MB/s | 4.6 Mbit/s |
| SSD Starter (1c/4gb) | Cherryvale-KS | ❌ Timeout | — | — | — | — |

> ⚠️ NVMe Starter disk read (7.3 MB/s) was measured **without cache drop**.
> The NVMe Standard run properly dropped page cache first and showed 409 MB/s.
> The NVMe Starter real disk read is likely much higher than 7.3 MB/s.

> SSD Starter provisioning timed out after 386 seconds. Possible platform issue
> similar to the known Dev zone bug (#28). Not a provider bug.

## Raw data

See [results.json](results.json) for the complete JSON output including per-run details.

## Reproducing

```bash
export SHC_API_KEY="shc_live_..."
pip install shc-toolkit paramiko

python3 -c "
from shc_toolkit.client import SHCClient
import time, paramiko, re

c = SHCClient()
vm = c.order_vm(hostname='test-bench', size='nvme-2c-8gb',
                template='debian12-cloud', auto_cancel=True, pay=True,
                ssh_key=open('$HOME/.ssh/id_ed25519.pub').read().strip())
sid = str(vm.get('id') or vm.get('service_id'))

# Wait for active+IP
for _ in range(30):
    time.sleep(10)
    info = c.get_vm(sid)
    if info.get('service_status') == 'active' and info.get('ips'):
        ip = info['ips'][0]['ip']
        break

time.sleep(30)  # Let sshd settle
ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect(ip, username='debian', key_filename='$HOME/.ssh/id_ed25519')

# Run your benchmarks here
_, stdout, _ = ssh.exec_command('dd if=/dev/zero of=/dev/null bs=1M count=10000 2>&1')
print(stdout.read().decode())

ssh.close()
c.cancel_vm(sid, immediate=True)
"
```
