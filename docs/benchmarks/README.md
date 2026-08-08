# SHC VM Performance Benchmarks

**Date**: 2026-08-08
**Provider version**: terraform-provider-shc v0.2.0+
**Methodology**: sysbench 1.0.20 (CPU), fio 3.33 (4K random + sequential disk I/O)

## ⚠️ Preliminary Results Disclaimer

These are **preliminary, single-run benchmarks** on SHC NVMe VPS plans. Results reflect a
specific point in time and are affected by:

- **Shared hardware**: VPS plans share physical hosts. Neighbor noise (noisy neighbors)
  affects performance. A single run cannot capture this variance.
- **Single run**: No averaging across multiple runs. Production-grade benchmarks use
  5-10 runs with median reporting.
- **No network test this run**: iperf3/curl network benchmarks were not included due to
  time constraints. See prior results in git history for network data.
- **No random read test**: Only random write was tested. Random read is equally important
  for database workloads.
- **Geographic limitation**: All VMs are in Katy, TX. No cross-zone comparison.

For production planning, run your own benchmarks with:
```bash
sudo apt-get install -y sysbench fio iperf3
sysbench --test=cpu --cpu-max-prime=20000 run
fio --name=rw --ioengine=libaio --iodepth=1 --rw=randwrite --bs=4k --direct=1 --size=64M --runtime=60 --time_based
```

## Results

| Plan | Location | CPU (sysbench) | 4K Random Write | Sequential Write |
|------|----------|---------------|-----------------|-----------------|
| NVMe Starter (1c/4gb) | Katy-TX | 1,097 events/s | 3,555 IOPS (13.9 MB/s) | 96.6 MB/s |
| NVMe Standard (2c/8gb) | Katy-TX | 1,126 events/s | 3,461 IOPS (13.5 MB/s) | 107.0 MB/s |

### Context for interpreting these numbers

- **CPU ~1,100 events/s**: This is the sysbench CPU prime benchmark. For context, a dedicated
  modern x86 core typically scores 2,000-4,000 events/s. VPS shared cores score lower due to
  CPU time sharing. Both SHC plans perform similarly (~1,100), suggesting the same physical
  CPU is backing both — the 2c plan gets more scheduled time but not a faster clock.

- **4K random write ~3,500 IOPS**: This is the most important metric for database workloads.
  3,500 IOPS is solid for an NVMe-backed VPS — comparable to mid-range dedicated SSDs.
  Both plans show similar IOPS, suggesting the same underlying NVMe storage.

- **Sequential write ~100 MB/s**: Good for file operations. The NVMe Standard is slightly
  faster (107 vs 97 MB/s), likely due to more available CPU for I/O processing.

## What changed from prior benchmarks

Prior benchmarks used `dd` (memory bandwidth proxy for CPU, no cache-drop for disk).
These results use proper industry-standard tools:
- **sysbench** for CPU (actual prime calculation, not memory copy)
- **fio** for disk (direct I/O, proper queue depth, runtime-based)

## Reproducing

```bash
export SHC_API_KEY="shc_live_..."
pip install shc-toolkit paramiko

python3 -c "
from shc_toolkit.client import SHCClient
import time, paramiko

c = SHCClient()
vm = c.order_vm(hostname='test-bench', size='nvme-2c-8gb',
                template='debian12-cloud', auto_cancel=True, pay=True,
                ssh_key=open('$HOME/.ssh/id_ed25519.pub').read().strip())
sid = str(vm.get('id') or vm.get('service_id'))

for _ in range(30):
    time.sleep(10)
    info = c.get_vm(sid)
    if info.get('service_status') == 'active' and info.get('ips'):
        ip = info['ips'][0]['ip']; break

time.sleep(40)
ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect(ip, username='debian', key_filename='$HOME/.ssh/id_ed25519')
ssh.exec_command('sudo apt-get update -qq && sudo apt-get install -y -qq sysbench fio')
time.sleep(25)

# CPU
_, o, _ = ssh.exec_command('sysbench --test=cpu --cpu-max-prime=10000 run 2>&1')
print(o.read().decode())

# Disk 4K random write
_, o, _ = ssh.exec_command('fio --name=rw --ioengine=libaio --iodepth=1 --rw=randwrite --bs=4k --direct=1 --size=64M --runtime=20 --time_based --group_reporting 2>&1')
print(o.read().decode())

ssh.close()
c.cancel_vm(sid, immediate=True)
"
```

Raw JSON results: [results.json](results.json)
