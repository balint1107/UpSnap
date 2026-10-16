# Power button response handling

Wake and shutdown endpoints return HTTP 200 with a device record whose `status`
is `online` or `offline`. The frontend checks HTTP success through `Response.ok`,
then displays the returned device state. Comparing the record's status to the
number 200 incorrectly reverted successful actions and left cards stale.

Failed actions show the server's error and reload the actual device record.
Run the regression tests with:

`node --experimental-strip-types --test frontend/tests/power-response.test.mjs`

For the home deployment, the shutdown command suspends the host. Verify both the
card and the actual host through a wake/suspend cycle without refreshing the page.

Configure `ping_cmd` as a restricted SSH readiness check. Wake remains pending
until that command succeeds, and routine status reflects usable SSH access.
Shutdown uses a separate ICMP reachability probe so an SSH service stopping alone
cannot complete the suspend action. It remains pending until the host stops replying.
Run `go test ./networking -run TestSuspendProbeIgnoresReadinessCommand` in `backend`
for the regression check separating readiness from suspend completion.

The production frontend build and targeted regression tests are required. The
upstream `pnpm check` currently reports a missing Node `async_hooks` declaration
in generated Paraglide server code; this is independent of the power-state fix.
