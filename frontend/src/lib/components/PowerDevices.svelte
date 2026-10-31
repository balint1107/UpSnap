<script lang="ts">
	import { deviceCategories, type Plug } from '$lib/types/inventory';
	import { deviceInventory, refreshDevice } from '$lib/stores/inventory';
	import ServiceAvailability from './ServiceAvailability.svelte';
	import { faChevronDown, faPlug } from '@fortawesome/free-solid-svg-icons';
	import Fa from 'svelte-fa';
	import PowerChart from './PowerChart.svelte';
	let { search = '' }: { search?: string } = $props();
	const visible = $derived(
		$deviceInventory.devices.filter(
			(d) =>
				d.driver === 'appliance' &&
				`${d.name} ${d.address}`.toLowerCase().includes(search.toLowerCase())
		)
	);
	/** Format nullable physical value in unit with decimals places; unavailable values remain dashes. */
	function metric(value: number | null | undefined, decimals: number, unit: string) {
		return value == null ? '—' : `${value.toFixed(decimals)} ${unit}`;
	}
</script>

{#if visible.length}<h2 class="appliance-heading">{deviceCategories.home_appliances}</h2>{/if}
{#each visible as common (common.id)}
	{@const device: Plug = common.appliance ?? { device: common, id: common.driver_id, name: common.name, ip: common.address,
        status: 'unknown', detail: 'Readings not yet available.', checked_at: '',
        reading: {power_w: null, energy_today_kwh: null, energy_month_kwh: null, voltage_v: null, current_a: null,
            relay_on: null, model: '', hardware: '', firmware: ''}, history: [], daily: [] }}
	{@const failed = common.failed}
	{@const fresh = !!common.appliance && !failed && common.state === 'online'}
	<details
		class="server-card device-row bg-base-100 power-device"
		data-power-device={device.id}
		data-inventory-id={common.id}
		data-category={common.category}
		data-state={failed ? 'stale' : common.state}
	>
		<summary class="device-row-summary">
			<span class="row-host-name"><Fa icon={faPlug} /> {common.name}</span>
			<span class="row-host-ip">{common.address}</span>
			<span class="host-state" class:ready={fresh}
				>{fresh
					? device.reading.relay_on == null
						? 'Available'
						: device.reading.relay_on
							? 'Outlet on'
							: 'Outlet off'
					: !common.appliance && !failed
						? common.refreshing
							? 'Checking…'
							: 'Unknown'
						: failed || common.state === 'stale'
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
				<button
					class="btn btn-sm"
					onclick={() => refreshDevice(common.id)}
					disabled={common.refreshing}>Refresh</button
				>
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
				Checked {new Date(common.checked_at ?? 0).toLocaleString('en-GB', {
					timeZone: 'Europe/Budapest'
				})}{#if device.reading.firmware}
					· Firmware {device.reading.firmware}{/if}
			</div>
		</div>
		<ServiceAvailability deviceId={common.id} embedded />
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
	.power-detail > p {
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
