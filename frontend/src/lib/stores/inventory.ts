import { browser } from '$app/environment';
import { get, readable } from 'svelte/store';
import { backendUrl, pocketbase } from './pocketbase';
import type { DeviceEnvelope } from '$lib/types/inventory';
type InventoryState = { devices: DeviceEnvelope[]; failed: boolean };
/** Poll read-filtered metadata for visible subscribed dashboards, without initiating SSH probes. */
export const deviceInventory = readable<InventoryState>({ devices: [], failed: false }, (set) => {
	if (!browser) return;
	let devices: DeviceEnvelope[] = [];
	set({ devices, failed: false });
	let pending = false;
	let stopped = false;
	let request: AbortController | undefined;
	/** Fetch current-user metadata; retain old data on failure and never publish after unsubscribe. */
	async function refresh() {
		if (pending || document.visibilityState !== 'visible') return;
		pending = true;
		request = new AbortController();
		try {
			const response = await fetch(`${backendUrl}api/upsnap/devices`, {
				headers: { Authorization: get(pocketbase).authStore.token },
				signal: request.signal
			});
			if (!response.ok) throw new Error('Device inventory unavailable');
			const value = await response.json();
			if (value.schema_version !== 1) throw new Error('Unsupported inventory version');
			devices = value.devices;
			if (!stopped) set({ devices, failed: false });
		} catch (error) {
			if (!stopped && !(error instanceof DOMException && error.name === 'AbortError'))
				set({ devices, failed: true });
		} finally {
			pending = false;
		}
	}
	const timer = window.setInterval(refresh, 30000);
	document.addEventListener('visibilitychange', refresh);
	void refresh();
	/** Abort pending reads and release polling after the final subscriber leaves. */
	return () => {
		stopped = true;
		clearInterval(timer);
		request?.abort();
		document.removeEventListener('visibilitychange', refresh);
	};
});
