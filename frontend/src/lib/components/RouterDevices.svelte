<script lang="ts">
	import { deviceCategories, type DeviceEnvelope } from '$lib/types/inventory';
	import ServiceAvailability from './ServiceAvailability.svelte';
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import { resolve } from '$app/paths';
	import { backendUrl, pocketbase } from '$lib/stores/pocketbase';
	import { faChevronDown, faTerminal } from '@fortawesome/free-solid-svg-icons';
	import Fa from 'svelte-fa';
	type Band = { id: string; name: string; on: boolean | null };
	type Router = {
		device: DeviceEnvelope;
		id: string;
		name: string;
		ip: string;
		model: string;
		status: string;
		bands: Band[];
		leds: boolean | null;
		led_control: boolean;
		led_scope: string;
		lcd: boolean;
		lcd_on: boolean | null;
		firmware: string;
		uptime: string;
		load: string[];
		cpu: string | null;
		memory_total: number | null;
		memory_free: number | null;
		checked_at: number;
	};
	let { search = '' }: { search?: string } = $props();
	let routers: Router[] = $state([]);
	let canControl = $state(false);
	let failed = $state(false);
	let pending = $state('');
	let error = $state('');
	let fetching = $state(false);
	let abort: AbortController | undefined;
	let requestVersion = 0;
	const visible = $derived(
		routers.filter((r) =>
			`${r.name} ${r.ip} ${r.model}`.toLowerCase().includes(search.toLowerCase())
		)
	);
	/** Fetch authenticated router diagnostics; force requests bypass the shared cache at most once every 2s. */
	async function refresh(force = false) {
		if (fetching || pending) return;
		fetching = true;
		const version = requestVersion;
		abort = new AbortController();
		try {
			const response = await fetch(`${backendUrl}api/upsnap/routers${force ? '?refresh=1' : ''}`, {
				headers: { Authorization: get(pocketbase).authStore.token },
				signal: abort.signal
			});
			if (!response.ok) throw new Error('Networking unavailable');
			const result = await response.json();
			if (version !== requestVersion) return;
			routers = result.routers;
			canControl = result.can_control;
			failed = false;
		} catch (e) {
			if (version === requestVersion && !(e instanceof DOMException && e.name === 'AbortError'))
				failed = true;
		} finally {
			fetching = false;
		}
	}
	/** Apply a supported on/off action to router id; update switches only from observed response, never optimistically. */
	async function change(id: string, action: string, on: boolean, band = '') {
		if (pending || !canControl) return;
		pending = id;
		requestVersion++;
		error = '';
		try {
			const response = await fetch(`${backendUrl}api/upsnap/routers/${id}/${action}`, {
				method: 'POST',
				headers: {
					Authorization: get(pocketbase).authStore.token,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ value: on ? 'on' : 'off', band })
			});
			if (!response.ok) throw new Error('Switch failed; refresh to check actual state.');
			const result = await response.json();
			routers = routers.map((r) => (r.id === id ? result : r));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Router unavailable';
		} finally {
			pending = '';
		}
	}
	/** Poll only while visible and cancel the current diagnostic read on navigation. */
	onMount(() => {
		/** Refresh router metadata after tab visibility returns; hidden tabs do not poll. */
		const resume = () => {
			if (document.visibilityState === 'visible') void refresh();
		};
		void refresh();
		const timer = setInterval(resume, 30000);
		document.addEventListener('visibilitychange', resume);
		return () => {
			clearInterval(timer);
			document.removeEventListener('visibilitychange', resume);
			abort?.abort();
		};
	});
	/** Format nullable switch state; unknown remains explicit. */
	function switchState(value: boolean | null) {
		return value == null ? 'Unknown' : value ? 'On' : 'Off';
	}
	/** Format Linux uptime seconds as days/hours/minutes; preserve RouterOS duration strings. */
	function uptime(value: string) {
		const seconds = Number(value);
		if (!value) return '—';
		if (!Number.isFinite(seconds)) return value;
		return `${Math.floor(seconds / 86400)}d ${Math.floor(seconds / 3600) % 24}h ${Math.floor(seconds / 60) % 60}m`;
	}
	/** Format bytes as MiB or an unavailable dash. */
	function memory(bytes: number | null) {
		return bytes == null ? '—' : `${(bytes / 1048576).toFixed(1)} MiB`;
	}
</script>

<section class="networking" aria-label="Networking devices">
	<h2>{deviceCategories.networking}</h2>
	{#if error}<p role="alert">{error}</p>{/if}
	{#if failed}<p role="status">Router diagnostics unavailable. Previous readings are stale.</p>{/if}
	{#each visible as router (router.id)}
		{@const ready = !failed && router.device.state === 'online'}
		<details
			class="server-card device-row bg-base-100 router-row"
			data-router={router.id} data-inventory-id={router.device.id} data-category={router.device.category}
			data-state={failed ? 'stale' : router.device.state}
		>
			<summary class="device-row-summary">
				<span class="row-host-name">{router.name}</span><span class="row-host-ip">{router.ip}</span>
				<span class="host-state"
					>{pending === router.id
						? 'Updating…'
						: failed || router.device.state === 'stale'
							? 'Stale'
							: ready
								? 'SSH ready'
								: 'Unavailable'}</span
				>
				<span class="compact-metrics router-metrics"
					><span
						>Wi-Fi <strong
							>{ready
								? router.bands.length
									? `${router.bands.filter((b) => b.on).length}/${router.bands.length}`
									: '—'
								: '—'}</strong
						></span
					><span
						>{router.lcd && !router.device.capabilities.includes('leds') ? 'LCD' : 'LED'}
						<strong
							>{ready
								? router.lcd && !router.device.capabilities.includes('leds')
									? switchState(router.lcd_on)
									: router.device.capabilities.includes('leds')
										? switchState(router.leds)
										: '—'
								: '—'}</strong
						></span
					></span
				>
				<span class="row-service-count">Router</span>
				<span class="row-terminal"
					>{#if ready}<a
							class="btn host-power host-icon-button"
							href={resolve('/terminal') + `/?device=${router.id}`}
							aria-label={`Open terminal for ${router.name}`}
							title="Terminal"
							onclick={(e) => e.stopPropagation()}
							onkeydown={(e) => e.stopPropagation()}><Fa icon={faTerminal} /></a
						>{/if}</span
				>
				<span class="row-chevron" aria-hidden="true"><Fa icon={faChevronDown} /></span>
			</summary>
			<div class="router-detail">
				<div class="router-heading">
					<div>
						<h3>{router.model}</h3>
						<p>{router.firmware || 'Firmware unavailable'}</p>
					</div>
					<button class="btn btn-sm" onclick={() => refresh(true)} disabled={!!pending || fetching}
						>Refresh</button
					>
				</div>
				<div class="router-controls">
					{#each router.bands as band}<div class="switch-item">
							<span
								>Wi-Fi · {band.name}<small>{ready ? switchState(band.on) : 'Unavailable'}</small
								></span
							><button
								type="button"
								role="switch"
								aria-label={`${router.name} Wi-Fi ${band.name}`}
								aria-checked={band.on === true}
								class="router-switch"
								class:on={band.on === true}
								disabled={!ready || !canControl || !!pending}
								onclick={() => change(router.id, 'wifi', band.on !== true, band.id)}
								><span></span></button
							>
						</div>{/each}
					<div class="switch-item">
						<span
							>LEDs<small
								>{router.led_scope}{#if router.device.capabilities.includes('leds')}
									· {ready ? switchState(router.leds) : 'Unavailable'}{/if}</small
							></span
						><button
							type="button"
							role="switch"
							aria-label={`${router.name} LEDs`}
							aria-checked={router.leds === true}
							class="router-switch"
							class:on={router.leds === true}
							disabled={!ready || !canControl || !!pending || !router.device.capabilities.includes('leds')}
							onclick={() => change(router.id, 'leds', router.leds !== true)}><span></span></button
						>
					</div>
					{#if router.lcd}<div class="switch-item">
							<span
								>LCD display<small>{ready ? switchState(router.lcd_on) : 'Unavailable'}</small
								></span
							><button
								type="button"
								role="switch"
								aria-label={`${router.name} LCD display`}
								aria-checked={router.lcd_on === true}
								class="router-switch"
								class:on={router.lcd_on === true}
								disabled={!ready || !canControl || !!pending}
								onclick={() => change(router.id, 'lcd', router.lcd_on !== true)}
								><span></span></button
							>
						</div>{/if}
				</div>
				<div class="diagnostics">
					<span>Uptime <strong>{uptime(router.uptime)}</strong></span><span
						>{router.cpu != null ? 'CPU' : 'Load 1/5/15m'}
						<strong
							>{router.cpu != null ? `${router.cpu}%` : router.load?.join(' / ') || '—'}</strong
						></span
					><span
						>Memory free / total <strong
							>{memory(router.memory_free)} / {memory(router.memory_total)}</strong
						></span
					>
				</div>
				<p class="checked">
					Checked {new Date(router.device.checked_at ?? 0).toLocaleString('en-GB', {
						timeZone: 'Europe/Budapest'
					})}{#if router.id === 'home-asus'}
						· Wi-Fi state follows the configured radio policy.{/if}
				</p>
			</div>
		<ServiceAvailability deviceId={router.device.id} embedded />
		</details>
	{:else}<p>
			{failed
				? 'Networking access unavailable.'
				: fetching
					? 'Loading networking devices…'
					: 'No matching networking devices.'}
		</p>{/each}
</section>

<style>
	.networking {
		margin-top: 2rem;
	}
	h2 {
		font-size: 1.1rem;
		font-weight: 650;
		margin-bottom: 0.75rem;
	}
	.router-row {
		margin-bottom: 0.75rem;
	}
	.router-metrics {
		display: flex;
		gap: 0.75rem;
		font-size: 0.75rem;
		color: var(--monitor-muted);
	}
	.row-terminal :global(.btn) {
		width: 40px;
		height: 40px;
		padding: 0;
		font-size: 12px;
	}
	.router-detail {
		padding: 1.25rem;
	}
	.router-heading {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		align-items: center;
	}
	h3 {
		font-size: 0.95rem;
		font-weight: 600;
	}
	p,
	small {
		color: var(--monitor-muted);
		font-size: 0.75rem;
	}
	.router-controls {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 0.75rem;
		margin-block: 1rem;
	}
	.switch-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		border: 1px solid var(--monitor-border);
		border-radius: 8px;
		padding: 0.85rem;
		font-size: 0.85rem;
	}
	small {
		display: block;
		margin-top: 0.2rem;
	}
	.router-switch {
		background: #94a3b8;
		width: 36px;
		height: 20px;
		border-radius: 20px;
		padding: 3px;
		flex-shrink: 0;
	}
	.router-switch span {
		display: block;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: white;
		transition: transform 0.15s;
	}
	.router-switch.on {
		background: #386a99;
	}
	.router-switch.on span {
		transform: translateX(16px);
	}
	.router-switch:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.router-switch:focus-visible {
		outline: 2px solid #386a99;
		outline-offset: 3px;
	}
	.diagnostics {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem 2rem;
		font-size: 0.8rem;
	}
	.checked {
		margin-top: 1rem;
	}
	strong {
		color: var(--monitor-ink);
	}
</style>
