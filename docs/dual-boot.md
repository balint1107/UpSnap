# Kiertekelo dual-boot switching

One physical device has pinned Ubuntu/Windows SSH profiles configured through
`UPSNAP_BOOT_HOSTS_FILE`; `boot_managed=true` displays its current OS and a
single expandable OS card matching the device rows. Its header contains the
selected OS, inline Boot entry and Validate/Restart buttons, including when
collapsed. Expanding the full-width header shows only Ubuntu/Windows choices
and their Boot entries. Choosing an OS updates the header and closes the list;
action buttons do not toggle it. Progress and messages sit outside the choices.
Checkmarks identify the authenticated running OS, independently of target selection.
The header check appears only when its chosen target is currently running; the
dropdown check stays on the running OS. Checks are hidden while offline, unknown,
or switching. Highlighting and aria-pressed still identify the chosen restart target.
Selection defaults to the detected OS and remains
unchanged during background refreshes. Terminal defaults follow the detected OS; interactive SSH
still requires the user's password. Windows resource/suspend commands remain
unsupported; Ubuntu stats/suspend and offline WoL remain available.
Unavailable OS readings clear stale metrics. Suspend completion uses raw network
reachability independently of SSH, so a stopped SSH daemon cannot prove suspend.

## Controls and authorization

- GET `/api/upsnap/boot/{id}` requires device-read permission. It returns no-store
  firmware status and the latest persisted switch job, including failures.
- POST `/api/upsnap/boot/{id}/validate` requires device-power permission and a
  bounded `{"target":"ubuntu"}` or `{"target":"windows"}` body; it is read-only.
- POST `/api/upsnap/boot/{id}/prepare` is a maintenance operation that additionally
  requires Pi/host `writes_enabled`. It refuses existing BootNext, writes only
  the selected BootNext and verifies read-back plus unchanged raw BootOrder.
- POST `/api/upsnap/boot/{id}/restart` additionally requires Pi/host
  `restart_enabled`, installed helpers for both OS profiles and a confirmation
  in the UI. It persists a per-host job before preparing BootNext, verifies source
  boot identity, then requests the native graceful restart. Returns HTTP 202.
- The legacy `/reboot/{id}` route stays blocked for dual-boot devices; it cannot
  bypass the OS-selection workflow. Unsupported actions and locked gates fail
  closed. Detection and errors never trigger WoL, suspend, forced power or retries.

Helpers restrict SSH to fixed actions and the Pi's source address. Root-owned
Linux and administrator/SYSTEM-owned Windows configuration controls host gates;
unprivileged Linux execution forcibly disables writes and restarts. New installs
start locked. This deployment was explicitly authorized to enable both gates.

## Progress and failure behavior

A job records actor ID, target, source OS/boot ID, original raw BootOrder and OS
loader order before any mutation. Writes use atomic rename and fsync under a
per-host lock in `pb_data/boot-jobs/`. A second switch is rejected while active.
Status reads reconcile progress without replaying a restart, including after a
monitor restart. The browser polls every three seconds during a switch and every
30 seconds otherwise. It reports ready only after pinned SSH authenticates the
expected OS with a changed boot ID, consumed BootNext and preserved OS-loader
order; timeout is six minutes. Failed preparation never starts a restart. A lost
acknowledgement or timeout requires review of BootNext before another attempt.

Firmware drops legacy BBS CD/DVD/network entries on Windows boots and may recreate
them on Ubuntu boots. Compare the Ubuntu/Windows loader order across OS changes;
raw BootOrder must remain exactly unchanged within preparation/queueing. No tool
rewrites BootOrder or GRUB defaults. A loader validation alone does not prove
that the target OS boots or that its SSH is ready.

Ubuntu uses a 15-second systemd timer followed by ordinary `systemctl reboot`.
Windows uses a 15-second SYSTEM scheduled task followed by `shutdown /r /t 0`
without `/f`: positive shutdown timeouts imply forced application termination,
so only Task Scheduler supplies the delay. Applications may block the graceful
restart; the monitor times out rather than forcing closure or cutting power.

## Verification and deployment

Go tests use fake SSH and isolated databases to check authentication, power
permissions, gates, source identity, queue acknowledgements, persistent progress,
readiness, timeouts and loader-order changes. Python tests use temporary fake EFI
files and an injected runner; they never write real firmware or restart anything.
The frontend production build passes; type checking retains a pre-existing
generated Paraglide `async_hooks` error.

See home-automation's `monitor/boot/README.md` for host installation, live
round-trip results, firmware mapping, rollback and Windows RDP setup. Never use
legacy reboot or forced power as a fallback for a failed OS switch.
