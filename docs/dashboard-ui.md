# Server dashboard appearance

The home-server fork uses neutral light/dark themes (`corporate` and `business`),
a simple server mark and account icon, compact bordered cards, explicit SSH-ready
and offline indicators, and accessible text buttons labelled Wake and Suspend.
Pending actions display a plain countdown until the existing backend probes
confirm completion. Decorative avatars, mascot defaults and card scale animations
are removed. Mobile layouts stack cards and summary information without scrolling
horizontally. Power handling and access permissions are retained.

Saved web apps use the navy server logo, with 192/512px PNG, maskable and
180px Apple touch icons. Manifest icons, launch URL, scope and app ID resolve
relative to the manifest, preserving `/monitor/` instead of using another app
at the domain root. Uploaded custom icons remain supported. macOS/iOS may retain
an installed icon: remove the old web app/shortcut and add it again to refresh.
Verified production build, backend tests, PNG dimensions and live manifest/icon
URLs. The HTML manifest link is relative during prerendering and resolves to a
stable app URL in the browser, including after SPA login redirects.

Each card includes live CPU, memory and local disk usage, collected only while
SSH is ready. Browser requests run every 15 seconds; the backend shares a 15-second
memory cache between sessions. The authenticated `/api/upsnap/stats/{id}` endpoint
uses the device's read permission and returns unavailable without running SSH for
offline/pending devices. Collection uses the fixed `stats` SSH command configured
by `UPSNAP_STATS_SSH_CONFIG`. No metrics history or additional database writes are
introduced. A previous reading remains visibly marked as stale after disconnect.

Expanded resources also display available GPU diagnostics per device: model,
utilization, VRAM usage, temperature and power. The restricted host collector uses
a two-second `nvidia-smi` query, with read-only DRM/sysfs fallback for counters
provided by other drivers (including Nouveau temperature). Unsupported fields
remain null/dashes; hosts without GPU counters omit the section. GPU failures do
not disable CPU/RAM/disk readings. GPU data shares the existing refresh/cache and
stale indication; no persistent metrics history is added.

GPU verification: collector tests cover multiple NVIDIA devices, unsupported
values, CLI timeout, no GPU and AMD/sysfs units. Backend tests and production
build passed; live forced-SSH/API readings and 1440px/390px layouts were checked.
The pre-existing generated Paraglide `async_hooks` type-check error remains.

Read the deployment's `docs/monitor.md` for how to rebuild the frontend and embed
it into the ARM64 binary. Verify desktop/mobile rendering and server status against
the backend after deployment. Existing tabs need one refresh to load new assets.

Deployment checks on 2026-10-16 passed for desktop light/dark, 390px mobile
without horizontal overflow, valid readings on both servers, cache reuse,
unchanged device timestamps, and anonymous/unauthorized access rejection.
The backup card tracked pending -> SSH ready -> pending -> offline without a
page reload; a direct SSH connection confirmed readiness after wake.

Card descriptions do not grow to fill spare height, so offline and online hosts
keep their IP/MAC rows aligned. The pencil beside each name opens a name-only
editor. `POST /api/upsnap/rename/{id}` accepts a trimmed 1–100 character name;
it requires device read access plus power or update permission (or superuser).
Other submitted fields are ignored, so operators cannot change addresses or
commands through this endpoint. Cancel leaves the name unchanged.

Verified the mixed online/offline layout with a browser-only fixture: both IP
rows had exactly the same vertical coordinate. Mobile remained within 390px.
Operator rename survived reload; cancel, original-name restoration, invalid-name
rejection, ignored non-name fields and anonymous/read-only denials were checked.
No power commands were issued for these checks.

While the dashboard is visible it also reloads device states every 30 seconds,
and immediately when returning to the tab. This supplements realtime updates;
wake/suspend actions continue tracking completion immediately. The periodic
refresh stops while hidden and is removed, along with this page's realtime
subscriptions, on navigation. Older responses cannot overwrite newer requests.
Resource readings retain their separate 15-second refresh interval.

Browser timer checks passed: one device-list refresh over a visible 31-second
window, none over a hidden 31-second window, and one immediately on return.

Devices with `reboot_only` have no wake/suspend/sleep controls. Individual APIs
reject those operations, group operations skip them, and network helpers reject
them for scheduled calls too. Their reboot uses a fixed `reboot` action through
the restricted SSH config and returns 202/pending. The host helper delays a true
system reboot by five seconds so the response and database state can be saved.
For the Pi hosting UpSnap, service interruption is expected; startup probes
restore current states after the host returns. Reboot requires confirmation.

Pi deployment verification covered three visible hosts, live Pi readings, API
denials without state changes, forced-helper rejection, and the reboot dialog's
notice and Cancel action. Unit tests cover the fixed SSH reboot command and
early wake/suspend/sleep rejection; a real host reboot was not triggered.

The Services section reads `/api/upsnap/services` every 30 seconds while visible.
A Pi systemd collector checks the HTTPS application routes, router DNS repair and
network filtering every 30 seconds independently of open browsers. The container
only reads its atomic JSON snapshot from `UPSNAP_SERVICES_FILE`; it has no router
administration key or Docker socket. Results older than 90 seconds become stale.
Each service is visible only with read permission for its host. Unreachable
applications on hosts with unavailable SSH are labelled Host offline. Disabled
filtering is distinct from degraded enforcement or an unknown router state.

Deployment verified six fresh service results, anonymous denial, per-host read
isolation and empty results for users without host access. Stale/future/corrupt
snapshot tests passed. Desktop/mobile layouts passed at 390px without overflow;
the Pi timer kept publishing independently of browser requests.

Devices now use collapsed, keyboard-accessible disclosure rows with name, IP,
SSH state and service count. Expanding a row shows controls, resource readings
and the services belonging to that host. Resource collection serves the compact
summary even while details are collapsed.
Devices and Services are separate navigation tabs (`/` and `/services/`). The
Services tab groups checks by host. One shared frontend store refreshes service
results every 30 seconds regardless of how many rows are expanded. Web services
use the service name as the only link, opening a new tab with noopener;
checks without a web UI remain unlinked. Only HTTP(S) service URLs are rendered.

The network-filtering card includes current enforcement, the configured daily
window/timezone and next on/off transition, or an explicit paused/off schedule.
Service details preserve line breaks while retaining the same compact card size.

The separate Open service/URL line has been removed. Browser verification found
one name link per web service and confirmed the MLflow name opens its real page.

Row/tab verification passed: three initially collapsed rows, correct per-host
services, one shared service request when opening multiple rows, resource widget
removal on collapse, keyboard expansion and mobile menu navigation at 390px.
The Services tab showed three host groups; Open MLflow opened its real URL in a
new browser tab. Production build passed; no power action was used for checks.

Service grids retain empty column tracks (`auto-fill`) so singleton host groups
use the same card width as larger groups. Every card is 9rem high with its
timestamp at the bottom; unusually long content scrolls within the card. Mobile
uses one full-width column with the same consistent height.

Verified all six desktop cards at matching column widths and equal height,
including singleton groups; timestamps aligned and content did not overflow.
The 390px mobile layout also retained matching dimensions without overflow.

Card height was reduced from 18rem to 14rem, with tighter status spacing.
All six cards measured 196px on desktop/mobile without content overflow.

The name-only link layout is further compacted to 9rem cards with 0.8rem padding,
smaller grid/group gaps and tighter metadata spacing. Text sizes are retained.
All six cards measured 126px after deployment on desktop and 390px mobile,
without content overflow.

Embedded service cards on Devices use the same compact content layout as the
Services tab. Host name/IP appear in group/device headings, not inside cards. Metadata/status paragraphs explicitly disable flex
growth so enclosing device-card styles cannot stretch their spacing.
Browser comparison confirmed identical height, padding, font sizes and content
offsets between embedded and standalone cards; mobile cards also fit without overflow.

Device summary rows include live CPU/RAM/root-disk percentages and power controls
without opening details. Wake/suspend keep their confirmation and pending-state
handling; the Pi offers reboot only. Controls stop click/key propagation so they
do not toggle the disclosure. Each host has one resource collector shared by the
compact and expanded views; online collapsed rows now collect every 15 seconds.
Offline summaries show dashes rather than stale values. Expanded readings reuse
the same snapshot and do not create a second request/SSH collector.

Compact-row checks passed on desktop and 390px mobile without overflow. Opening
details issued no duplicate metrics request. Reboot/suspend confirmation and
Cancel left rows collapsed, and no power requests were sent during verification.
The three power-response regression tests and production build passed.

Refresh is now a labelled information action: it reloads the device record,
current resource reading, and shared service snapshot without sending a power
command. Reboot is a separate labelled button beside Wake/Suspend, retaining its
existing confirmation and permissions. All hosts expose Reboot only in expanded
details. Row strips contain only Wake/Suspend; reboot-only hosts have no power
action on their strip. The strip keeps its compact single-button width.

Browser checks confirmed Reboot confirmation/Cancel in details, no row reboot actions,
record/resource/service requests from Refresh, and no power API calls during the
checks. Layouts fit at 390, 768, 1001, 1200 and 1440px. The overview subtitle now
reflects public passkey access and LAN/VPN-only password sign-in. Production build
passed; the existing generated Paraglide type-check error remains.

Routine pages omit explanatory subtitles, service-collector implementation notes,
and passkey tutorials. The initial stock device descriptions are cleared in the
home deployment; custom descriptions remain supported. Action confirmations,
errors, live status/freshness indicators, and the public login's short LAN/VPN
password-access explanation remain available. Operational details stay in docs.


Connected device rows include a terminal icon in a reserved column, preserving
IP alignment for offline rows. It opens a separate xterm.js page with a fresh
SSH username/password prompt, responsive PTY sizing, and mobile control keys.
Any signed-in user can use it; the SSH host still authenticates every session.


Device rows use matching 40px bordered power and terminal icon buttons. The power
symbol indicates online, the moon indicates offline/suspended, and a spinner
marks a pending action;
accessible names and hover labels retain the action and device name. The pending
hover label includes its countdown. Reboot remains labelled in expanded details.
Reserved control columns, a centered chevron icon and fixed row heights at each
responsive breakpoint keep reboot-only, online, offline and pending rows aligned.

Power/terminal glyphs are 12px and chevrons 10px, with unchanged 40px button
targets. Mobile actions share a bottom control row. Browser checks cover uniform
row heights and IP/chevron alignment at 390, 768, 1000, 1001, 1200 and 1440px;
offline/pending fixtures issue no power requests. Production build passes; the
previously documented generated Paraglide type-check error remains.


The terminal page fills the available visual viewport instead of adding a fixed
65vh terminal below a tall form. Small screens use two-column login fields,
16px login/terminal inputs to avoid automatic browser zoom, and two rows of touch
control keys.
Connected sessions collapse the disabled login fields; Login expands them again,
while Disconnect remains visible. The visual viewport accounts for software
keyboard changes and triggers PTY resizing. Portrait and landscape share this
compact mode; login and password-only SSH authentication remain unchanged.

Verified production build and browser layouts at 320x568, 390x844, 844x390 and
1440x900. A browser-only connected-session fixture covered Login expansion,
Disconnect restoration, PTY resize frames and a reduced 390x400 viewport. These
checks do not simulate the actual iOS keyboard. No real machine password or power
action was used for the layout checks. The generated Paraglide type-check error
remains unchanged.

Dual-boot hosts show an authenticated OS badge within the existing device row.
Expanded details use one collapsible OS card matching device rows. Its header
contains the selected OS, Boot entry, Validate/Restart buttons and chevron.
The expanded list contains only OS choices and their Boot entries; selecting one
closes the list. Buttons act independently of expansion; progress appears outside
the choices. Checkmarks identify the authenticated running OS, not the selected
restart target, and disappear while offline, unknown or switching. The selected
target remains highlighted. Background status refreshes
preserve an explicit selection. Authorized, enabled Restart choices require confirmation
and track the expected OS's SSH readiness; ordinary Reboot is omitted for managed
hosts. Switching disables conflicting power actions. See `docs/dual-boot.md`.


## Read-only smart-plug devices

The P115 now appears as its own disclosure row on Devices, with IP, outlet state,
watts and today's kWh. Expansion shows voltage/current, monthly energy and two
responsive SVG charts: sampled power over 24h and observed daily energy over 30d.
The plug has no SSH or relay controls and is no longer listed under Services.
Search includes the plug name/IP; host summaries continue counting SSH hosts.

`/api/upsnap/power` reads the Pi's `power.json` next to the service snapshot,
requires authentication and the collecting host's read permission, bounds input
to 1 MiB/8 devices/1441 samples/30 days and marks old/future readings stale.
The UI polls at 30s while visible and aborts on navigation. Failures retain history
with stale status and hide current values. History is sampled from activation,
not fabricated or backfilled; daily totals are partial observations and today is
incomplete. Hovering plotted points/bars reveals recorded values; collection gaps
over two minutes are left disconnected. No chart dependency was added.
See deployment `docs/monitor.md` for retention, storage and the private collector.

Verified production ARM64 deployment with real P115 readings, persistent samples,
Devices/Services placement and 1440px/390px layouts. Backend/networking tests,
15 collector tests and production frontend build pass. Full Svelte checking still
reports the existing generated Paraglide `async_hooks` type error only.


Chart refinement: plots now use the full measured card width with fixed-size
11px labels and a compact 190px plotting area, avoiding SVG letterboxing and
oversized desktop labels. Power uses padded, rounded bounds around observed
readings (a nonzero baseline is possible); energy bars retain zero. Axes have
regular numeric ticks with 0.1 W / 0.001 kWh labels and intermediate time ticks.
The displayed time range fits available history up to the retention window,
so a newly enabled monitor does not compress samples against the right edge.
Scale tests cover small variations, zero/flat data, invalid values and bar baselines.

Verified against real history at 1440px/390px: full-width plots, decimal labels,
unique daily bar labels and no horizontal overflow. Four scale tests, frontend
production build and backend/networking suites passed. No power commands were issued.


## Device categories and router controls

Devices is divided into Computers (the existing three SSH hosts), Networking
(four configured routers) and Home appliances (P115). Each router uses the same
compact disclosure-row layout and password-terminal icon. Its dropdown includes
firmware, uptime, CPU/load, memory, radio/LED state, Refresh and supported toggles.
Unsupported MikroTik radios/LED blackout are omitted or disabled; its LCD has a
separate switch. The older ASUS explicitly labels its LED switch Power LED only
and its Wi-Fi readback as firmware policy, not a hardware measurement.

Router status inherits Pi read permission; mutation additionally requires Pi
power permission and validates fixed IDs/actions/bands in both Go and the Pi
helper. No arbitrary shell command is accepted. Switches show pending state and
update from the observed response. Shared 30-second status polling pauses when
hidden; failures mark retained values stale and disable controls. Router terminals
retain independent password-only login for signed-in users, pinned host keys and
session limits. Per-router custom SSH ports and legacy algorithms are explicit,
leaving the computers' Ed25519 policy intact. See deployment
`monitor/routers/README.md` for setup, hardware limits and restoration checks.


Networking verification: the live page contains 3 computer rows, 4 router rows
and 1 appliance row, with four router terminal links and no 390px overflow.
A real browser LED cycle confirmed off/on readback and restored on. Anonymous
router APIs return 401; unsupported/injected switches return 400. A temporary
read-only account could read diagnostics but not mutate (403), while signed-in
terminal metadata remained available without device grants. Test accounts were
removed. Real pinned SSH/password-method negotiation passed for all four routers;
actual user-password shell login remains a private user check. Five controller
unit tests and backend/networking suites pass; production build passes and full
Svelte checking retains only the generated `async_hooks` error. Snapshot cache age
starts at sampling time so 30s polls do not repeatedly reuse old readings; reads
started before a toggle cannot overwrite its observed response.

A browser-only delayed-read fixture confirmed an older GET cannot overwrite a
completed toggle; the fixture made no hardware changes and was removed before
returning to live readings. Router terminal buttons reuse the computer row's
40px icon-button style. All three section headers and router diagnostics were
visually checked on desktop and mobile.

A live 31-second check observed the router snapshot timestamp advance by 30s;
all four routers remained online with tested radio/indicator states restored.

## Shared device metadata

The dashboard consumes version-one common envelopes (`docs/device-schema.md`) for
computer capability metadata and router/appliance state/support. Section labels share
one type contract. Services group by stable `device_id`, including embedded cards in
router/appliance details, rather than IP comparisons. Driver-specific rows, charts,
password terminals and action authorization remain in place. Legacy driver payloads
are preserved for compatibility; inventory polling is visible-page-only every 30s and
reads cached metadata, without adding SSH probes.
