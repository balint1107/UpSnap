import { browser } from '$app/environment';
import { get, readable } from 'svelte/store';
import { backendUrl, pocketbase } from './pocketbase';
import type { DashboardDevice, DeviceEnvelope, Plug, Router } from '$lib/types/inventory';
import { reconcileInventory } from '$lib/helpers/inventory';

type InventoryState = {
	devices: DashboardDevice[];
	loaded: boolean;
	failed: boolean;
	unavailableDrivers: string[];
};
let refresh = () => {};
let refreshOne: (id: string, force: boolean) => void = () => {};
let control: (
	id: string,
	action: string,
	on: boolean,
	band: string
) => Promise<void> = async () => {};

/** Share authorized device definitions and independent observations while any dashboard subscribes. */
export const deviceInventory = readable<InventoryState>(
	{ devices: [], loaded: false, failed: false, unavailableDrivers: [] },
	(set) => {
		if (!browser) return;
		let value: InventoryState = {
			devices: [],
			loaded: false,
			failed: false,
			unavailableDrivers: []
		};
		let stopped = false;
		let inventoryQueued = false;
		let session = 0;
		const requests = new Map<string, AbortController>();
		/** Publish changes to the current authorized session, unless its final subscriber has left. */
		function publish(changes: Partial<InventoryState>) {
			value = { ...value, ...changes };
			if (!stopped) set(value);
		}
		/** Update a known device by stable id; removed or unauthorized rows cannot be resurrected. */
		function update(id: string, changes: Partial<DashboardDevice>) {
			publish({ devices: value.devices.map((d) => (d.id === id ? { ...d, ...changes } : d)) });
		}
		/** Read a path using the current token and abort signal; non-2xx responses reject without private diagnostics. */
		async function read(path: string, signal: AbortSignal) {
			const response = await fetch(`${backendUrl}api/upsnap/${path}`, {
				headers: { Authorization: get(pocketbase).authStore.token },
				signal
			});
			if (!response.ok) throw new Error('Device readings unavailable');
			return response.json();
		}
		/** Collect one router or the shared power snapshot; force bypasses only the router cache. */
		async function details(id: string, force = false) {
			const device = value.devices.find((d) => d.id === id);
			if (!device || device.driver === 'computer' || document.visibilityState !== 'visible') return;
			const key = device.driver === 'appliance' ? 'power' : id;
			if (requests.has(key)) return;
			const targets = value.devices.filter((d) =>
				device.driver === 'appliance' ? d.driver === 'appliance' : d.id === id
			);
			const currentSession = session;
			const controller = new AbortController();
			requests.set(key, controller);
			/** True only while this request still owns the current session's observation slot. */
			const current = () =>
				!stopped && currentSession === session && requests.get(key) === controller;
			for (const target of targets) update(target.id, { refreshing: true });
			try {
				const result = await read(
					device.driver === 'router'
						? `routers/${encodeURIComponent(device.driver_id)}${force ? '?refresh=1' : ''}`
						: 'power',
					controller.signal
				);
				if (!current()) return;
				for (const target of targets) {
					const reading: Router | Plug | undefined =
						device.driver === 'router'
							? result
							: result.devices.find((p: Plug) => p.device.id === target.id);
					if (!reading || reading.device.id !== target.id) {
						update(target.id, { failed: true });
						continue;
					}
					// Inventory owns identity and permissions; a diagnostic response supplies observations only.
					update(target.id, {
						state: reading.device.state,
						checked_at: reading.device.checked_at,
						failed: false,
						...(device.driver === 'router'
							? { router: reading as Router }
							: { appliance: reading as Plug })
					});
				}
			} catch {
				if (current() && !controller.signal.aborted)
					for (const target of targets) update(target.id, { failed: true });
			} finally {
				if (requests.get(key) === controller) {
					requests.delete(key);
					if (!stopped && currentSession === session)
						for (const target of targets) update(target.id, { refreshing: false });
				}
			}
		}
		/** Fetch inventory first, publishing rows before starting independent device diagnostics. */
		async function inventory() {
			if (stopped || !get(pocketbase).authStore.isValid || document.visibilityState !== 'visible')
				return;
			if (requests.has('inventory')) {
				inventoryQueued = true;
				return;
			}
			const controller = new AbortController();
			requests.set('inventory', controller);
			const currentSession = session;
			try {
				const result = await read('devices', controller.signal);
				if (result.schema_version !== 1) throw new Error('Unsupported inventory version');
				if (stopped || currentSession !== session) return;
				publish({
					devices: reconcileInventory(value.devices, result.devices as DeviceEnvelope[]),
					loaded: true,
					failed: false,
					unavailableDrivers: result.unavailable_drivers ?? []
				});
				for (const d of value.devices) void details(d.id);
			} catch {
				if (!stopped && currentSession === session && !controller.signal.aborted)
					publish({ failed: true, loaded: true });
			} finally {
				if (requests.get('inventory') === controller) {
					requests.delete('inventory');
					if (inventoryQueued) {
						inventoryQueued = false;
						void inventory();
					}
				}
			}
		}
		/** Execute a supported router action and merge observed state; older reads cannot overwrite it. */
		async function action(id: string, name: string, on: boolean, band: string) {
			const device = value.devices.find((d) => d.id === id);
			if (!device || device.driver !== 'router' || !device.permissions.control || device.refreshing)
				return;
			const currentSession = session;
			requests.get(id)?.abort();
			requests.delete(id);
			const controller = new AbortController();
			requests.set(id, controller);
			update(id, { refreshing: true });
			try {
				const response = await fetch(
					`${backendUrl}api/upsnap/routers/${encodeURIComponent(device.driver_id)}/${name}`,
					{
						method: 'POST',
						signal: controller.signal,
						headers: {
							Authorization: get(pocketbase).authStore.token,
							'Content-Type': 'application/json'
						},
						body: JSON.stringify({ value: on ? 'on' : 'off', band })
					}
				);
				if (!response.ok) throw new Error('Switch failed; refresh to check actual state.');
				const reading: Router = await response.json();
				if (!stopped && currentSession === session && reading.device.id === id)
					update(id, {
						router: reading,
						state: reading.device.state,
						checked_at: reading.device.checked_at,
						failed: false
					});
			} catch (error) {
				if (!stopped && currentSession === session) update(id, { failed: true });
				throw error;
			} finally {
				if (requests.get(id) === controller) {
					requests.delete(id);
					if (!stopped && currentSession === session) update(id, { refreshing: false });
				}
			}
		}
		refresh = () => {
			void inventory();
		};
		refreshOne = (id, force) => {
			void details(id, force);
		};
		control = action;
		/** Return the signed-in collection/record identity; an invalid session has no reusable data. */
		function authIdentity() {
			const auth = get(pocketbase).authStore;
			return auth.isValid ? `${auth.record?.collectionName}:${auth.record?.id}` : '';
		}
		let identity = authIdentity();
		/** Drop all prior-user state and requests, then reload the new authenticated session. */
		const stopAuth = get(pocketbase).authStore.onChange(() => {
			const next = authIdentity();
			if (identity === next) return;
			identity = next;
			session++;
			for (const controller of requests.values()) controller.abort();
			requests.clear();
			inventoryQueued = false;
			publish({ devices: [], loaded: false, failed: false, unavailableDrivers: [] });
			void inventory();
		});
		const timer = window.setInterval(refresh, 30000);
		document.addEventListener('visibilitychange', refresh);
		publish(value);
		refresh();
		/** Abort reads, remove listeners and discard session data when the final subscriber leaves. */
		return () => {
			stopped = true;
			session++;
			stopAuth();
			clearInterval(timer);
			document.removeEventListener('visibilitychange', refresh);
			for (const controller of requests.values()) controller.abort();
			refresh = () => {};
			refreshOne = () => {};
			control = async () => {};
		};
	}
);

/** Request current authorized inventory and subsequent diagnostic refreshes; returns immediately. */
export function refreshInventory() {
	refresh();
}
/** Refresh the observation for stable id; force bypasses the router cache within its server rate limit. */
export function refreshDevice(id: string, force = false) {
	refreshOne(id, force);
}
/** Apply action to a stable router id; on is desired state, band is optional radio ID; rejects on API failure. */
export function controlRouter(id: string, action: string, on: boolean, band = '') {
	return control(id, action, on, band);
}
