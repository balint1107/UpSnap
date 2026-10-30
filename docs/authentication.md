# Dashboard authentication

The HTTPS login form identifies its username and current-password inputs with
standard `name` and `autocomplete` attributes for Apple Passwords and other
password managers. The username field continues accepting email or username.

Password AutoFill, passkeys (WebAuthn), and Sign in with Apple (OAuth2) are
different login methods. UpSnap already lists configured PocketBase OAuth2
providers in its login menu; enabling OAuth2 without provider credentials does
not add a usable identity provider. Existing password access remains available.

## Passkeys (Apple Passwords)

Set `UPSNAP_WEBAUTHN_RP_ID=bgamgee.ddns.net` and
`UPSNAP_WEBAUTHN_ORIGIN=https://bgamgee.ddns.net` on the monitor process.
Both are required; without them passkey routes are disabled. The origin must be
HTTPS and match the configured RP hostname. It has no `/monitor` suffix.
Never derive these values from request headers. Passkey login may be exposed publicly;
password and recovery authentication have a separate backend network policy.

On LAN/VPN, sign in with the regular user password, open **Account**, and choose **Add passkey**.
Select Apple Passwords in the system prompt and confirm with Touch ID/Face ID.
Subsequent visits can use **Sign in with passkey**, without typing the username.
Use the same HTTPS hostname when enrolling and signing in. A secure browser context
and a WebAuthn-capable browser are required. Superuser accounts retain password login;
use a regular user account for daily passkey access. Removing a passkey in Account
revokes its future logins, but does not remove its local copy from Apple Passwords or
revoke existing sessions. Keep a recovery password. Enrollment requires an authenticated
regular-user session; protect that session as you would access to account settings.

The maintained `go-webauthn/webauthn` library verifies challenges, RP ID, origin,
signatures, and required user verification. Discoverable credentials are required.
The `passkeys` collection stores public credentials only, partitioned by RP ID and
owned by a user; its ordinary REST rules are locked. Custom list/delete routes expose
only the signed-in user's metadata. Superusers can recover accounts through PocketBase.
Successful sign-in uses PocketBase's normal authentication response, including auth
rules/MFA hooks. Challenges are single-use, account/purpose-bound, expire after five
minutes, and are bounded to 1024 pending ceremonies. Restarts cancel pending ceremonies.
Each account can register up to ten passkeys. Back up PocketBase `pb_data` to preserve
registered public credentials; changing the RP hostname requires fresh enrollment.

Verification: `CGO_ENABLED=0 go test ./...` in `backend`, production frontend build,
and a browser with a resident, user-verifying virtual authenticator. The virtual
flow verifies protocol integration; actual Apple Passwords enrollment needs the user's
system prompt confirmation. The existing generated Paraglide `async_hooks` type-check
failure is documented in `docs/power-controls.md`.

The deployed browser verification covered enrollment, a resident credential,
username-less sign-in, and rejection of assertion replay and a signed assertion
without user verification. API checks covered account isolation, locked collection
access, anonymous enrollment denial, and untrusted origins. Temporary test accounts
and credentials are removed after verification. Unit tests also preserve assertion
bodies across WebAuthn verification and PocketBase auth hooks, reject oversized or
invalid JSON, and check ceremony expiry, ownership, purpose, replay, and capacity.

## Public passkeys and private password authentication

`UPSNAP_PASSWORD_ALLOWED_CIDRS` is a comma-separated list of networks allowed to
use password, OTP, OAuth2, and password-reset/recovery routes. When unset, generic
UpSnap deployments retain existing behavior. The home deployment allows LAN
`192.168.50.0/24`, VPN `10.8.0.0/24`, and explicit Pi-local maintenance addresses.
`UPSNAP_TRUSTED_PROXY_CIDRS` identifies only the nginx socket peer (`172.18.0.3/32`
currently). Only that peer may supply `X-Real-IP`; nginx overwrites it with
`$remote_addr` and overwrites `X-Forwarded-For`. Other peers' forwarded headers are
ignored. Missing or malformed trusted client addresses fail closed. Verify the
proxy container IP after recreation and update the trusted peer if needed; an IP
change denies password sign-in until repaired, while passkey sign-in continues.

A global backend middleware checks authentication actions for every collection
name or ID, including `_superusers`. Blocking a specific `/users/` URL at nginx
alone is insufficient because PocketBase accepts collection IDs too. Public clients
see a passkey-only login page; `/api/upsnap/passkeys/config` returns the network
policy with `Cache-Control: no-store`. Hiding fields is presentation only; direct
password/recovery API attempts are rejected with 403 before authentication.
Authenticated sessions may refresh and use their existing permissions from any
network. Passkey enrollment/removal still require a valid regular-user session.
Recovery through a password requires returning to LAN/VPN. nginx must keep the
backend reachable only through its bridge listener, never a public port.

Tests cover LAN/VPN/public peers, IPv4-mapped addresses, malformed addresses,
forged forwarded headers, missing trusted headers, collection-ID routes, and
password/recovery denial without blocking passkey ceremonies or token refresh.

Deployment verification used the real WAN address and a resident virtual
authenticator: public passkey enrollment/sign-in and token refresh passed, password
attempts with correct credentials returned 403 (including collection IDs and forged
headers), and LAN password login returned 200. All temporary credentials/accounts
were removed and existing registered passkeys preserved.

## Browser SSH terminals

Any authenticated monitor user may open `/terminal/?device=<id>`; device power
permissions do not grant or restrict this separate SSH login. Each connection
requires an SSH username and password. The Go bridge uses password authentication
only, never monitor keys, an SSH agent, or a saved password. Passwords and monitor
tokens travel in the first same-origin WSS frame, never URLs or application logs.
Terminal content is not recorded by the monitor; host shell policies still apply.

Set `UPSNAP_TERMINAL_HOSTS` to comma-separated literal device IPs and
`UPSNAP_TERMINAL_KNOWN_HOSTS` to an existing OpenSSH known_hosts file. Only those
online device records on port 22 are reachable. Ed25519 host keys must already be pinned (the bridge explicitly negotiates Ed25519);
unknown/changed keys fail before password transmission. The configured
`UPSNAP_WEBAUTHN_ORIGIN` also supplies the exact allowed WebSocket origin. Empty
origin/host configuration disables terminals. nginx must forward WebSocket Upgrade
and Connection headers under the existing `/monitor/` prefix.

The first frame has a 10-second deadline and 8 KiB limit. Sessions have a
15-minute input-idle timeout, eight-hour maximum, two sessions per account,
16 active SSH sessions globally and 32 socket handshakes globally. Each account
may attempt six SSH logins per minute. Tokens are revalidated every 30 seconds;
logout/navigation closes the current browser terminal immediately. Closing a
socket releases the SSH channel. Resize and input frames are size bounded.
SSH password sign-in works wherever an authenticated monitor session is allowed;
the LAN/VPN restriction on the monitor's own password login remains unchanged.

Verification: backend tests cover password-only SSH with multiple offered host-key
types, rejection before credential transmission for unknown keys, PTY shell
input/output and resizing, origin checks, allowlisted destinations and capacity
limits. The frozen dependency install and production build pass. The only
frontend type-check failure remains the pre-existing generated Paraglide
`async_hooks` error. Live checks cover monitor authentication, WebSocket upgrade,
invalid-token rejection, anonymous page redirects, mobile layout and row IP alignment.
A temporary regular user without device grants could access terminal metadata for
all three hosts while its ordinary device list remained empty; it was removed. All three real hosts
negotiate the pinned Ed25519 key and reach password authentication. Successful
login with the user's private machine passwords requires user verification.

The terminal page retains separate SSH username/password fields and
Connect/Disconnect controls above the remote terminal. Authentication completes
before opening the remote shell. The password field is cleared when connecting;
SSH credentials are not entered through an emulated terminal login prompt.

On mobile and short landscape windows, connected sessions collapse the disabled
credential fields behind Login to leave room for the terminal and keyboard.
Disconnect stays available; Login expands the same username/password form.

Dual-boot status uses device-read permission; target validation/preparation uses
device-power permission. Restart also requires explicit Pi/host restart gates,
verified source identity and a persisted switch job. Dedicated
monitor keys use fixed forced-command helpers; terminals retain password-only
login. OS keys are pinned independently and the default terminal username follows
the authenticated OS. See `docs/dual-boot.md` for the independent write/restart gates.


Networking devices use trusted `UPSNAP_ROUTERS_FILE` metadata (default
`/app/ssh/routers.json`). Router status requires Pi read access; switches require
Pi power access. Controller SSH keys remain private on the Pi, reached only via
the existing forced-command key with bounded `router` requests. Router terminals
use typed SSH passwords exclusively, with the configured port, default username
and exact pinned host-key algorithm. RSA/ECDSA and group14-sha1 compatibility are
allowed only for configured legacy routers, never a fallback for computer hosts.
MikroTik's `always-allow-password-login` is enabled so terminals remain usable
alongside existing keys; its controller uses a separate limited-policy user.

## Central Authelia deployment

The home deployment now gates Monitor through Authelia. Optional `UPSNAP_SSO_FILE`
contains a private proxy secret, explicit Authelia-username → existing regular-user
ID bindings, and `admin_ids` for service administration. `GET /api/upsnap/sso`
requires that secret and a bound identity and returns the existing user's ordinary
PocketBase token. It never creates users, grants device permissions or maps emails
implicitly. Ordinary direct browser-supplied headers fail authentication. nginx must
overwrite identity headers and keep the backend restricted.

The frontend exchanges verified identity before refreshing authentication. In central
mode its account menu links to Authelia passkey settings, hides the independent
Monitor password/passkey editors and ends the central session on logout. Existing
Monitor credentials are preserved for recovery; central passkeys require enrollment.
Terminal SSH credentials remain independent.

Service-route administration uses a fixed filesystem-protected Unix socket via
`UPSNAP_SERVICE_ROUTES_SOCKET`. Device control does not confer administration;
only explicit administrator IDs or PocketBase superusers may call the management
API. Changes are validated/applied by the host helper, not by a container Docker
socket. See home-automation `docs/authelia.md` and `docs/service-access.md`.

Central logout preserves the current Monitor URL through Authelia's `xrd` query
parameter. After clearing the shared session, Authelia restores it to `rd` on the
sign-in page so login returns to the same Monitor page rather than the global
default destination. The destination is built from the current browser location.
