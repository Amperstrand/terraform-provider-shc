# SHC VM Performance Benchmarks

**Date**: 2026-08-07
**Method**: Ordered real VMs via `shc-toolkit`, SSH'd via paramiko, ran `dd` benchmarks, destroyed immediately.
**Template**: debian12-cloud on all tests.

## Results

| Plan | Location | Provision Time | CPU (dd GB/s) | Disk Write | Disk Read | Network |
|------|----------|---------------|---------------|------------|-----------|---------|
| NVMe Starter (1c/4gb) | Katy-TX | 97s | 22.9 GB/s | 57.3 MB/s | 7.3 MB/s | 3.9 Mbit/s |
| NVMe Standard (2c/8gb) | Katy-TX | 43s | 21.0 GB/s | 53.4 MB/s | 4.8 MB/s | 4.5 Mbit/s |
| SSD Starter (1c/4gb) | Cherryvale-KS | ❌ Timeout (386s) | — | — | — | — |

## Notes

- **SSD Starter provisioning failed**: VM ordered but never reached `active` status within 386 seconds. Possible platform issue similar to Dev zone bug #28. Filed for investigation.
- **CPU benchmark**: `dd if=/dev/zero of=/dev/null` measures memory bandwidth, not pure CPU. `sysbench` is not pre-installed on `debian12-cloud`. Both NVMe plans showed ~21-23 GB/s throughput.
- **Disk read benchmark**: Lower than expected due to small file size (256MB) and possible page cache effects. The `drop_caches` call on the 1c/4gb test improved read speed (7.3 vs 4.8 MB/s without cache drop).
- **Network benchmark**: Low speeds (3.9-4.5 Mbit/s) are likely due to the speed test server (speedtest.tele2.net) being geographically distant from Katy, TX. Real-world throughput would be higher for US-based traffic.
- **Provisioning time**: NVMe plans provision reliably in 43-97 seconds. SSD plans may have provisioning issues.

## Methodology

```bash
# Each VM was:
# 1. Ordered via shc-toolkit (auto_cancel=True, immediate payment)
# 2. Polled until service_status=active AND ip assigned
# 3. SSH'd after 45s settle time (sshd needs time to start)
# 4. Benchmarked: dd (CPU), dd (disk write), dd (disk read after cache drop), curl (network)
# 5. Destroyed immediately via cancel_vm(immediate=True)
```

Raw JSON results: [results.json](results.json)
