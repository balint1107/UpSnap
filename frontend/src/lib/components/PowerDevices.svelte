<script lang="ts">
	import { deviceCategories, type DeviceEnvelope } from '$lib/types/inventory';
	import ServiceAvailability from './ServiceAvailability.svelte';
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import { backendUrl, pocketbase } from '$lib/stores/pocketbase';
	import { faChevronDown, faPlug } from '@fortawesome/free-solid-svg-icons';
	import Fa from 'svelte-fa';
	import PowerChart from './PowerChart.svelte';
	type Plug = {
		device: DeviceEnvelope;
		id: string;
		name: string;
		ip: string;
		status: string;
		detail: string;
		checked_at: string;
		reading: {
			power_w: number | null;
			energy_today_kwh: number | null;
			energy_month_kwh: number | null;
			voltage_v: number | null;
			current_a: number | null;
			relay_on: boolean | null;
			model: string;
			hardware: string;
			firmware: string;
		};
		history: { t: number; w: number }[];
		daily: { date: string; kwh: number }[];
	};
	let { search = '' }: { search?: string } = $props();
	let devices: Plug[] = $state([]);
	let failed = $state(false);
	let fetching = $state(false);
	let abort: AbortController | undefined;
	const visible = $derived(
		devices.filter((d) => `${d.name} ${d.ip}`.toLowerCase().includes(search.toLowerCase()))
	);
	/** Fetch the authenticated public power snapshot; preserve history and mark it stale on failure. */
	async function refresh() {
		if (fetching) return;
		fetching = true;
		abort = new AbortController();
		try {
			const response = await fetch(`${backendUrl}api/upsnap/power`, {
				headers: { Authorization: get(pocketbase).authStore.token },
				signal: abort.signal
			});
			if (!response.ok) throw new Error('Power snapshot unavailable');
			devices = (await response.json()).devices;
			failed = false;
		} catch (error) {
			if (!(error instanceof DOMException && error.name === 'AbortError')) failed = true;
		} finally {
			fetching = false;
		}
	}
	/** Start visible-page polling at 30s; return cleanup to abort pending reads on navigation. */
	onMount(() => {
		/** Refresh after the document becomes visible; hidden pages perform no periodic reads. */
		const resume = () => {
			if (document.visibilityState === 'visible') void refresh();
		};
		void refresh();
		const timer = window.setInterval(resume, 30000);
		document.addEventListener('visibilitychange', resume);
		return () => {
			clearInterval(timer);
			document.removeEventListener('visibilitychange', resume);
			abort?.abort();
		};
	});
	/** Format nullable physical value in unit with decimals places; unavailable values remain dashes. */
	function metric(value: number | null | undefined, decimals: number, unit: string) {
		return value == null ? '—' : `${value.toFixed(decimals)} ${unit}`;
	}
</script>

{#if failed && devices.length === 0}<p class="power-error" role="status">
		Power device readings unavailable.
	</p>{/if}
{#if visible.length}<h2 class="appliance-heading">{deviceCategories.home_appliances}</h2>{/if}
{#each visible as device (device.id)}
	{@const fresh = !failed && device.device.state === 'online'}
	<details
		class="server-card device-row bg-base-100 power-device"
		data-power-device={device.id}
		data-inventory-id={device.device.id} data-category={device.device.category}
		data-state={failed ? 'stale' : device.device.state}
	>
		<summary class="device-row-summary">
			<span class="row-host-name"><Fa icon={faPlug} /> {device.name}</span>
			<span class="row-host-ip">{device.ip}</span>
			<span class="host-state" class:ready={fresh}
				>{fresh
					? device.reading.relay_on == null
						? 'Available'
						: device.reading.relay_on
							? 'Outlet on'
							: 'Outlet off'
					: failed || device.status === 'stale'
						? 'Stale'
						: 'Unavailable'}</span
			>
			<span class="compact-metrics power-metrics"
				><span>Power <strong>{fresh ? metric(device.reading.power_w, 1, 'W') : '—'}</strong></span
				><span
					>Today <strong>{fresh ? metric(device.reading.energy_today_kwh, 3, 'kWh') : '—'}</strong
					></span
				></span
			>
			<span class="row-service-count">Smart plug</span>
			<span class="row-chevron" aria-hidden="true"><Fa icon={faChevronDown} /></span>
		</summary>
		<div class="power-detail">
			<div class="power-facts">
				<span
					>Voltage <strong>{fresh ? metric(device.reading.voltage_v, 1, 'V') : '—'}</strong></span
				>
				<span
					>Current <strong>{fresh ? metric(device.reading.current_a, 3, 'A') : '—'}</strong></span
				>
				<span
					>This month <strong
						>{fresh ? metric(device.reading.energy_month_kwh, 3, 'kWh') : '—'}</strong
					></span
				>
				<button class="btn btn-sm" onclick={refresh} disabled={fetching}>Refresh</button>
			</div>
			{#if !fresh}<p role="status">
					{failed ? 'Cannot refresh readings. Previous history retained.' : device.detail}
				</p>{/if}
			<div class="power-charts">
				<PowerChart points={device.history.map((p) => ({ t: p.t, value: p.w }))} />
				<PowerChart
					kind="energy"
					points={device.daily.map((p) => ({
						t: Date.parse(`${p.date}T12:00:00Z`) / 1000,
						value: p.kwh
					}))}
				/>
			</div>
			<div class="power-foot">
				Checked {new Date(device.device.checked_at ?? 0).toLocaleString('en-GB', {
					timeZone: 'Europe/Budapest'
				})}{#if device.reading.firmware}
					· Firmware {device.reading.firmware}{/if}
			</div>
		</div>
	<ServiceAvailability deviceId={device.device.id} embedded />
	</details>
{/each}

<style>
	.appliance-heading {
		margin-top: 2rem;
		margin-bottom: 0.75rem;
		font-size: 1.1rem;
		font-weight: 650;
	}
	.power-device {
		margin-top: 0.75rem;
	}
	.row-host-name :global(svg) {
		display: inline-block;
		margin-right: 0.35rem;
		font-size: 0.8rem;
	}
	.power-metrics {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		font-size: 0.75rem;
		color: var(--monitor-muted);
	}
	strong {
		color: var(--monitor-ink);
	}
	.ready {
		color: #2d8064;
	}
	.power-detail {
		padding: 1.25rem;
	}
	.power-facts {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
		align-items: center;
		font-size: 0.85rem;
	}
	.power-facts button {
		margin-left: auto;
	}
	.power-charts {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 1rem;
		margin-top: 1rem;
	}
	.power-foot,
	.power-detail > p,
	.power-error {
		color: var(--monitor-muted);
		font-size: 0.75rem;
		margin-top: 1rem;
	}
	@media (max-width: 800px) {
		.power-charts {
			grid-template-columns: 1fr;
		}
	}
</style>
