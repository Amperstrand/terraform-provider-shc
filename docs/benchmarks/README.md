# SHC VM Performance Benchmarks

**Date**: 2026-08-09
**Provider version**: terraform-provider-shc v0.3.0
**Methodology**: sysbench 1.0.20 (CPU), fio 3.33 (4K random read/write + sequential write)

## ⚠️ Preliminary Results Disclaimer

These are **preliminary, single-run benchmarks** on SHC NVMe VPS plans in Katy, TX.
Results reflect a specific point in time and are affected by:

- **Shared hardware**: VPS plans share physical hosts. Neighbor noise affects performance.
- **Single run**: No averaging across multiple runs.
- **NVMe zone only**: SSD and Dev plans (Cherryvale, KS) were not benchmarked due to
  zone provisioning issues (issue #28 on shc-toolkit). These will be added when the zone recovers.
- **No network test**: iperf3 was not included in this run (speed test servers are in Europe,
  geographic distance from Katy, TX depresses results).

For production planning, run the included benchmark script:
```bash
SHC_API_KEY=shc_live_... ./scripts/benchmark.sh nvme-1c-4gb nvme-2c-8gb
```

## Results

| Plan | CPU (sysbench) | 4K Random Write | 4K Random Read | Sequential Write |
|------|---------------|-----------------|-----------------|-----------------|
| NVMe Starter (1c/4gb) | 1,088 events/s | 3,695 IOPS (14.4 MB/s) | 4,098 IOPS | 101.0 MB/s |
| NVMe Standard (2c/8gb) | 1,090 events/s | 3,191 IOPS (12.4 MB/s) | 3,661 IOPS | 122.0 MB/s |

### Context

- **CPU ~1,090 events/s**: Shared vCPU. Dedicated modern x86 cores score 2,000-4,000.
  Both plans score similarly, suggesting same physical CPU backing.
- **4K random write ~3,200-3,700 IOPS**: Solid for NVMe-backed VPS. Suitable for
  database workloads. Comparable to mid-range dedicated SSDs.
- **4K random read ~3,700-4,100 IOPS**: Read performance slightly exceeds write,
  consistent with NVMe characteristics.
- **Sequential write ~100-122 MB/s**: NVMe Standard is ~20% faster, likely due to
  more available CPU for I/O processing.

### Plans not tested

| Plan | Zone | Reason |
|------|------|--------|
| SSD Starter (1c/4gb) | Cherryvale-KS | Zone provisioning timeout (issue #28) |
| HDD Starter (1c/4gb) | Unknown | Not tested (cost control) |
| Dev Starter (1c/4gb) | Cherryvale-KS | Zone broken (issue #28) |

## Raw data

See [results.json](results.json) for the complete JSON output.

## Reproducing

```bash
export SHC_API_KEY="shc_live_..."
./scripts/benchmark.sh nvme-1c-4gb nvme-2c-8gb
```
