#!/usr/bin/env bash
set -euo pipefail

# SHC VM Performance Benchmark Script
# Usage: SHC_API_KEY=shc_live_... ./scripts/benchmark.sh [plan1] [plan2] ...
# Default: benchmarks nvme-1c-4gb and nvme-2c-8gb
# Cost: ~$0.01-0.02 per VM (SHC refunds unused hours)

PLANS=("${@:-nvme-1c-4gb nvme-2c-8gb}")
RESULTS_DIR="docs/benchmarks"
DATE=$(date -u +%Y-%m-%d)

if [ -z "${SHC_API_KEY:-}" ]; then
  echo "Error: SHC_API_KEY must be set"
  exit 1
fi

echo "=== SHC VM Benchmark — $DATE ==="
echo "Plans: ${PLANS[*]}"
echo ""

pip install -q shc-toolkit paramiko 2>/dev/null

python3 << PYTHON
import os, time, json, re, subprocess
from shc_toolkit.client import SHCClient
import paramiko

def run_cmd(ssh, cmd, timeout=60):
    _, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    stdout.channel.recv_exit_status()
    return stdout.read().decode(), stderr.read().decode()

c = SHCClient(api_key=os.environ["SHC_API_KEY"])
plans = """${PLANS[*]}""".split()
results = []

for size in plans:
    hostname = f"test-bench-{size.replace('-','')}"
    print(f"\n=== {size} ===")

    vm = c.order_vm(hostname=hostname, size=size, template="debian12-cloud",
                    ssh_key=os.path.expanduser("~/.ssh/id_ed25519.pub"),
                    auto_cancel=True, pay=True)
    sid = str(vm.get("id") or vm.get("service_id"))
    t0 = time.time()

    for _ in range(30):
        time.sleep(10)
        info = c.get_vm(sid)
        if info.get("service_status") == "active" and info.get("ips"):
            ip = info["ips"][0]["ip"]
            break
    else:
        print("  TIMEOUT")
        results.append({"size": size, "status": "timeout"})
        c.cancel_vm(sid, immediate=True)
        continue

    ready = int(time.time() - t0)
    print(f"  Ready: {ready}s, IP={ip}")
    time.sleep(40)

    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    ssh.connect(ip, username="debian", key_filename=os.path.expanduser("~/.ssh/id_ed25519"), timeout=30)

    run_cmd(ssh, "sudo apt-get update -qq && sudo apt-get install -y -qq sysbench fio iperf3 2>&1 | tail -1", timeout=90)

    bench = {}

    # CPU: sysbench
    out, _ = run_cmd(ssh, "sysbench --test=cpu --cpu-max-prime=10000 --threads=1 run 2>&1", timeout=30)
    m = re.search(r'events per second:\s+([\d.]+)', out)
    if m: bench["cpu_sysbench_events_per_sec"] = round(float(m.group(1)), 1)

    # Memory: sysbench
    out, _ = run_cmd(ssh, "sysbench --test=memory --memory-block-size=1M --memory-total-size=10G run 2>&1", timeout=30)
    m = re.search(r'Transfer rate:\s+([\d.]+) (\w+)/sec', out)
    if m: bench["memory_transfer_rate"] = f"{m.group(1)} {m.group(2)}/sec"

    # Disk: fio 4K random write
    out, _ = run_cmd(ssh, "fio --name=rw --ioengine=libaio --iodepth=1 --rw=randwrite --bs=4k --direct=1 --size=64M --runtime=20 --time_based --group_reporting 2>&1", timeout=40)
    m = re.search(r'IOPS=(\w+)', out)
    if m: bench["disk_4k_randwrite_iops"] = int(m.group(1).replace("k","000"))
    m = re.search(r'bw=([\d.]+)(\w+)/s', out)
    if m:
        v = float(m.group(1))
        if "KiB" in m.group(2): v *= 0.001024
        bench["disk_4k_randwrite_mbs"] = round(v, 1)

    # Disk: fio sequential write
    out, _ = run_cmd(ssh, "fio --name=sw --ioengine=libaio --iodepth=1 --rw=write --bs=1M --direct=1 --size=256M --group_reporting 2>&1", timeout=30)
    m = re.search(r'bw=([\d.]+)(\w+)/s', out)
    if m:
        v = float(m.group(1))
        if "KiB" in m.group(2): v *= 0.001024
        bench["disk_seqwrite_mbs"] = round(v, 1)

    # Disk: fio 4K random read
    out, _ = run_cmd(ssh, "fio --name=rr --ioengine=libaio --iodepth=1 --rw=randread --bs=4k --direct=1 --size=64M --runtime=20 --time_based --group_reporting 2>&1", timeout=40)
    m = re.search(r'IOPS=(\w+)', out)
    if m: bench["disk_4k_randread_iops"] = int(m.group(1).replace("k","000"))

    # Network: iperf3 to public server
    out, _ = run_cmd(ssh, "iperf3 -c bouygues.iperf.fr -p 5200 -t 10 -R --json 2>/dev/null | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d[\"end\"][\"sum_received\"][\"bits_per_second\"]/1e6)' 2>/dev/null || echo FAIL", timeout=20)
    out = out.strip()
    if out != "FAIL":
        bench["network_iperf3_mbits"] = round(float(out), 1)
    else:
        out, _ = run_cmd(ssh, 'curl -so /dev/null -w "%{speed_download}" http://speedtest.tele2.net/10MB.zip 2>/dev/null || echo 0', timeout=30)
        if float(out.strip() or "0") > 0:
            bench["network_curl_mbits"] = round(float(out.strip()) * 8 / 1e6, 1)

    ssh.close()
    c.cancel_vm(sid, immediate=True)

    result = {"size": size, "status": "ok", "ready_after_seconds": ready, "ip": ip, "benchmarks": bench}
    results.append(result)
    print(f"  CPU: {bench.get('cpu_sysbench_events_per_sec','?')} events/s")
    print(f"  4K randwrite: {bench.get('disk_4k_randwrite_iops','?')} IOPS")
    print(f"  4K randread: {bench.get('disk_4k_randread_iops','?')} IOPS")
    print(f"  Seq write: {bench.get('disk_seqwrite_mbs','?')} MB/s")
    print(f"  Network: {bench.get('network_iperf3_mbits', bench.get('network_curl_mbits','?'))} Mbit/s")

# Save
output = {
    "date": "$DATE",
    "methodology": "sysbench CPU+memory, fio 4K random read/write + sequential write, iperf3/curl network. Tools installed via apt-get. 1 run per plan.",
    "results": results
}
with open("$RESULTS_DIR/results.json", "w") as f:
    json.dump(output, f, indent=2)

print(f"\n{'='*60}")
print(f"  BENCHMARKS COMPLETE — {len(results)} plans")
print(f"{'='*60}")
for r in results:
    b = r.get("benchmarks", {})
    print(f"  {r['size']:20s} CPU={b.get('cpu_sysbench_events_per_sec','?')}  4Kw={b.get('disk_4k_randwrite_iops','?')}IOPS  4Kr={b.get('disk_4k_randread_iops','?')}IOPS  seq={b.get('disk_seqwrite_mbs','?')}MB/s  net={b.get('network_iperf3_mbits', b.get('network_curl_mbits','?'))}Mbit/s")
PYTHON
