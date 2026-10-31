# Common devices and service references (version 1)

The authenticated `GET /api/upsnap/devices` endpoint returns `schema_version: 1`,
`checked_at` (UTC inventory assembly time), `devices` and `unavailable_drivers`.
It adapts the existing PocketBase computer records, trusted router definitions/cache
and read-only power snapshots. It performs no SSH probes and no hardware mutations. Router observations use an
immutable cache snapshot, so a slow in-flight router probe cannot block unrelated
inventory or service reads.
Optional driver failures are reported without hiding accessible computers. Existing
command/diagnostic endpoints and their permission checks remain authoritative.

Each common entry has:

| Field | Meaning |
| --- | --- |
| `id` | Stable namespaced identifier: `computer:<record-id>`, `router:<configured-id>`, `appliance:<configured-id>` |
| `driver`, `driver_id` | Adapter and original ID used by its existing API |
| `category` | `computers`, `networking`, `home_appliances` |
| `name`, `address` | Mutable display name/address; never relation keys |
| `collector_device_id` | Optional stable computer reference supplying collector permissions |
| `state` | `online`, `offline`, `pending`, `unknown`, `stale`, `degraded`, `disabled` |
| `checked_at` | Actual observation UTC RFC3339 timestamp, or null when unavailable |
| `capabilities` | Supported feature names; independent of current state and permission |
| `permissions` | Read/control grants for this caller; not a substitute for action authorization |

Capabilities currently include `resources`, `terminal`, `wake`, `suspend`, `reboot`,
`boot`, `wifi`, `leds`, `lcd`, `power_metrics`, `power_history`. Computer read/control
grants come from existing relations; routers inherit their configured collecting
computer's grants. A plug inherits collector read permission and has no control grant.
Terminal sessions retain their independent signed-in-user/password-only policy.
Computer checked_at is null because legacy host probes do not persist their observation
time; inventory assembly time must not be mistaken for probe freshness. Router/plug
observations older than 90s or over 10s in the future are stale. No observation means
unknown for an unsampled router; relay off does not mean a plug is unreachable.

`GET /api/upsnap/routers`, router action responses and `GET /api/upsnap/power` now
include a `device` envelope per item. Original diagnostic payloads/fields are preserved.
The dashboard shares category labels and capability metadata; service cards group by
`device_id` across all categories. Driver components retain specialized controls/charts.
This is a shared contract, not a rewrite of the existing storage into one database table.

Service configuration and exported snapshots support `device_id`. The service API
returns that stable relation, a normalized `state`, and current name/IP from inventory.
Legacy `status` and `host_ip` fields remain for older consumers. If no ID is present,
the backend resolves a legacy computer IP; an invalid/deleted/unauthorized explicit ID
never falls back to an IP. Services targeting routers/appliances inherit the referenced
device's visibility. Unknown references are omitted, not exposed as unowned checks.
The collector's `host_ip` remains a compatibility field; HTTP endpoint selection stays
in the check configuration. Missing optional-driver records hide only related services.

Pi configuration uses `UPSNAP_COLLECTOR_DEVICE_ID` and the plug snapshot's
`collector_device_id` to avoid IP-based permission inheritance. Pre-upgrade installs
may omit those fields and retain the previous Pi-IP/host-IP resolution. New deployments
must supply their own record IDs. No destructive database migration is involved.

For a new driver: choose a stable source ID, implement an envelope adapter with explicit
supported capabilities and permission host, and retain driver-specific diagnostics/actions
behind existing authorization. Add its detail component and contract tests; do not add
IP comparisons to frontend service grouping. Unknown hardware must not imply a control.

## Verification

Contract tests cover stable IDs across IP/name changes, invalid explicit-ID rejection,
read-only power capabilities, normalized freshness/UTC time, virtual-device service
references, read-permission isolation and nonblocking router cache access. Backend and
networking race tests plus Go vet pass; nine service/power collector tests and the
production frontend build pass. Full Svelte checking retains only the pre-existing
generated Paraglide `async_hooks` error.

Live verification on the Pi found 8 unique device IDs and 8 correctly related services.
Anonymous access was rejected; a temporary account with no grants saw empty results,
and Pi-only read access exposed exactly its six devices/five services with control false.
The account and grants were removed. Desktop/mobile rows and mobile service grouping
passed, with no horizontal overflow and the tools link available under Raspi. Hardware
power/radio/LED settings were not changed. The previous executable is hard-linked at
`monitor/build/upsnap.before-schema-20261026`; no database rewrite was needed.

## Inventory-first dashboard

All three device categories render from the authorized common inventory before
live diagnostics complete. `DashboardDevice` adds optional driver observations
and separate `refreshing`/`failed` flags to the existing envelope. Initial rows
show known names/addresses and Checking/unknown readings; refreshing keeps rows
and expanded router/appliance details in place. Failed reads retain observations
with a stale indication. Inventory refresh replaces permissions and removes
revoked devices; changing accounts clears observations and cancels pending reads.
Computer records and resource probes remain separate from inventory assembly.

`GET /api/upsnap/routers/{router}` reads one configured router independently,
using the same permissions and 29-second cache as the legacy batch endpoint.
`?refresh=1` retains a two-second minimum cache age. A per-router lock deduplicates
probes and orders them against action readback; only short cache publication is
shared across routers. Controls still serialize and return observed state.
The Pi controller must support `router status <id>` before using this frontend.

Optional `UPSNAP_APPLIANCES_FILE` (default `/app/ssh/appliances.json`) supplies
public appliance definitions independently of `power.json`. Each entry contains
`id`, `name`, `ip`, and `collector_device_id` (`computer:<record-id>`). Definitions
own identity/permissions; absent or corrupt measurements leave configured rows
unknown, with an appliance-driver warning. When the definitions file is absent,
legacy snapshot-derived inventory still works. Invalid definitions fail closed.
No credentials, controls, schema migration, or background hardware polling are added.

The inventory-first build was deployed on 2026-10-31 with the matching Pi
controller and public appliance definitions. Live API and deployed-browser checks
confirmed eight device rows before delayed diagnostics, independent router updates,
preserved expansion, anonymous denial and unchanged control grants. See the
home-automation deployment guide for verification details and rollback location.
