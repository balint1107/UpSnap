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
