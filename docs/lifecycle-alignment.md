# Lifecycle Semantics — Industry Alignment

How this provider maps SHC's service lifecycle onto the IaC conventions
established by AWS EC2 and Google Compute Engine, where they align, and —
more importantly — where SHC deliberately diverges and why. Written for users
arriving from other clouds and for reviewers auditing our design choices.

## Vocabulary

| Industry term | AWS EC2 | GCP GCE | SHC (API term) | This provider |
|---|---|---|---|---|
| Remove a resource permanently; billing ends | **terminate** (`TerminateInstances`) | **delete** | **cancel** (immediate) | `terraform destroy` → `Delete()` → `CancelVM(immediate)` |
| Pause without removing; instance kept, disks kept | **stop** (action resource) | `desired_status = "TERMINATED"` | stop / shutdown | `power_state = "stopped"` (attribute, GCP pattern) |
| Declared power state | — | `desired_status` | — | `power_state` |
| Observed state (drift detection) | `instance_state` | `current_status` | `service_status` + `provisioning_state` | `status`, `provisioning_state`, `ip` (computed) |
| Guard against destruction | `disable_api_termination` (+ `force_destroy` to override) | `deletion_policy = "PREVENT"` (select resources) | **confirm-gate**: destructive ops 409 with a single-use `confirmation_id`; provider auto-confirms only the operation Terraform explicitly requested | server-side confirm-gate + Terraform-native `lifecycle { prevent_destroy }` |
| Clean up forgotten test resources | provider **test sweepers** | — (sweeper pattern shared) | hourly **reaper** (`reap-orphan-vms.yml`, shc-toolkit `reap_orphans()`) | same, plus opt-in on-VM self-destruct timer |
| Billing period | per-second (running only) | per-second (running only) | **daily term**; renewed from account credit | `term` (days), `auto_cancel` |

## Design decisions and reasoning

### 1. `destroy` terminates (cancels) — never stops

`Delete()` calls `CancelVM(service_id, immediate=true)`: the service is
destroyed and the unused part of the current day is refunded. This matches
AWS (destroy → terminate) and GCP (destroy → delete). It is load-bearing for
SHC specifically because **SHC bills by existence**: a stopped VM accrues its
full daily price, so anything weaker than cancel on destroy would silently
keep charging (see divergence #5 below).

### 2. Power state is an attribute (GCP `desired_status` pattern), not an action resource

`power_state = "running" | "stopped"` is a mutable attribute; changing it
in-place triggers stop/start, without replacement and without a plan/apply
of a second resource. We chose GCP's model over AWS's newer "Action" resources
(`aws_ec2_stop_instance`, Terraform 1.14+): one resource owns one lifecycle,
power diffs show up in `terraform plan`, and there is no action-resource
ordering problem. Trade-off accepted: Terraform reconciles *desired* state,
so out-of-band power changes are reverted on the next apply — same trade-off
GCP users make with `desired_status`.

### 3. No `deletion_protection` attribute

AWS needs `disable_api_termination`/`force_destroy` because the raw EC2 API
has no destruction checkpoint. SHC has one server-side: every destructive
operation (cancel, reinstall, delete backup/snapshot/firewall rule/SSH key)
is confirm-gated — the first call returns 409 `confirmation_required` with a
single-use `confirmation_id`, and the provider re-sends the identical request
with `X-User-Api-Confirm` only for the operation Terraform explicitly
requested in the plan. Config-side protection stays with Terraform's native
`lifecycle { prevent_destroy }`. A third, provider-specific flag would be
redundant surface.

### 4. Ephemeral by default (`auto_cancel = true`)

Cloud default is renewal-by-default; ours is **destroy-at-term-by-default**.
Deliberate divergence, earned from incidents (VMs left stopped for days,
still billing; see shc-toolkit AGENTS.md lessons). A VM managed here does not
survive its paid term unless you explicitly opt into renewal
(`auto_cancel = false`). The end-of-term cancellation is scheduled only after
the VM reaches active+IP — scheduling it before the order invoice settles
voids the invoice and wedges the service in `pending` forever (SHC
live-earned contract).

### 5. Stopped ≠ free (the big divergence)

| | Compute charges while stopped |
|---|---|
| AWS EC2 (stopped) | **stop** — only EBS volumes keep billing |
| GCP GCE (`TERMINATED`) | **stop** — only disks / reserved IPs keep billing |
| **SHC (`power_state = "stopped"`)** | **continues at FULL daily price** — SHC charges for the service existing, not for it running |

`stop`/`shutdown` are a pause, not a cost control. Only cancel (destroy)
stops billing. The provider warns at apply time when entering `stopped`, and
the schema description carries the warning at plan time.

### 6. Readiness = `service_status == "active"` + IP assigned

AWS waiters block on `running`; GCP on `PROVISIONING → RUNNING`. SHC's
`provisioning_state` never reports `ready` on real VMs (documented lesson:
it is a claim, not proof), so the provider treats **active + assigned IP**
as provisioned, and cloud-init is explicitly *not* waited for (~120s lag).
Same philosophy as the operator discipline SHC publishes: a status field is
a claim; a reachable SSH endpoint is proof.

### 7. Orphan hygiene

Two layers, mirroring AWS's provider test **sweepers** and going one further:

- **Reaper** (hourly CI, shc-toolkit `reap_orphans()`): destroys orphaned
  VMs matching test hostname prefixes after a 2-hour age gate — the same
  job AWS provider sweeper runs perform, scheduled.
- **Self-destruct timer** (opt-in, `shc github-runner provision
  --self-destruct-minutes N`): an on-VM systemd timer cancels the VM N
  minutes after boot — closes the controller-dead gap (workflow cancelled
  mid-run) that external sweeping cannot. No mainstream cloud needs this
  because they bill per-second; SHC's daily-term billing makes the leak
  expensive enough to warrant it.

## Pulumi via the Terraform Bridge

The bridge inherits all of the above unchanged — that is the point of
bridging:

- `pulumi destroy` → provider `Delete()` → `CancelVM(immediate)` — billing ends.
- `power_state` maps to a mutable Pulumi property (`PowerState`); setting it
  to `"stopped"` triggers the same stop + billing warning.
- No Pulumi-specific lifecycle code exists or is needed; the deprecated
  native `shc-pulumi` provider implements the same semantics
  (`delete()` → `cancel_vm(immediate=True)`) but receives no new features.
