import type { DeviceState } from '$lib/types/inventory';
import { browser } from '$app/environment';
import { get, writable } from 'svelte/store';
import { backendUrl, pocketbase } from './pocketbase';

export type Service = {
	device_id: string;
	state: DeviceState;
	id: string;
	name: string;
	host_name: string;
	host_ip: string;
	url?: string;
	status: string;
	detail: string;
	latency_ms: number;
	checked_at: string;
};
type ServiceState = { services: Service[]; failed: boolean; fetching: boolean };
let trigger = () => {};
export const serviceStatus = writable<ServiceState>(
	{ services: [], failed: false, fetching: false },
	(set) => {
		if (!browser) return;
		let value: ServiceState = { services: [], failed: false, fetching: false };
		let request: AbortController | undefined;
		let stopped = false;
		const publish = (changes: Partial<ServiceState>) => {
			value = { ...value, ...changes };
			if (!stopped) set(value);
		};
		async function refresh() {
			if (value.fetching) return;
			publish({ fetching: true });
			request = new AbortController();
			try {
				const response = await fetch(`${backendUrl}api/upsnap/services`, {
					headers: { Authorization: get(pocketbase).authStore.token },
					signal: request.signal
				});
				if (!response.ok) throw new Error('Service checks unavailable');
				const result = await response.json();
				publish({ services: result.services, failed: false });
			} catch (error) {
				if (!(error instanceof DOMException && error.name === 'AbortError'))
					publish({ failed: true });
			} finally {
				publish({ fetching: false });
			}
		}
		trigger = () => {
			void refresh();
		};
		const visible = () => {
			if (document.visibilityState === 'visible') trigger();
		};
		const timer = window.setInterval(visible, 30000);
		document.addEventListener('visibilitychange', visible);
		trigger();
		return () => {
			stopped = true;
			trigger = () => {};
			window.clearInterval(timer);
			document.removeEventListener('visibilitychange', visible);
			request?.abort();
		};
	}
);
export function refreshServices() {
	trigger();
}
