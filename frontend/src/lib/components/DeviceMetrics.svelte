<script lang="ts">
	import { onMount } from 'svelte';
	import { backendUrl, pocketbase } from '$lib/stores/pocketbase';
	import type { Device } from '$lib/types/device';
	import type { HostMetrics } from '$lib/types/metrics';

	let {
		device,
		compact = false,
		collect = true,
		metrics = $bindable(),
		failed = $bindable(false)
	}: {
		device: Device;
		compact?: boolean;
		collect?: boolean;
		metrics?: HostMetrics;
		failed?: boolean;
	} = $props();
	let request: AbortController | undefined;
	let fetching = false;
	const rootDisk = $derived(metrics?.disks?.find((d) => d.mount === '/') ?? metrics?.disks?.[0]);
	const extraDisks = $derived(metrics?.disks?.filter((d) => d !== rootDisk) ?? []);
	const memoryPercent = $derived(metrics ? (100 * metrics.memory_used) / metrics.memory_total : 0);
	const diskPercent = $derived(rootDisk ? (100 * rootDisk.used) / rootDisk.total : 0);

	function size(bytes: number) {
		return `${(bytes / 1024 ** 3).toFixed(1)} GiB`;
	}
	async function refresh() {
		if (!collect || device.status !== 'online' || fetching) return;
		fetching = true;
		request = new AbortController();
		try {
			const response = await fetch(`${backendUrl}api/upsnap/stats/${device.id}`, {
				headers: { Authorization: $pocketbase.authStore.token },
				signal: request.signal
			});
			if (!response.ok) throw new Error('Metrics unavailable');
			const reading = (await response.json()) as HostMetrics;
			if (reading.available) metrics = reading;
			failed = false;
		} catch (error) {
			if (!(error instanceof DOMException && error.name === 'AbortError')) failed = true;
		} finally {
			fetching = false;
		}
	}
	$effect(() => {
		if (collect && device.status === 'online') void refresh();
		else request?.abort();
	});
	onMount(() => {
		if (!collect) return;
		const timer = window.setInterval(refresh, 15000);
		return () => {
			window.clearInterval(timer);
			request?.abort();
		};
	});
</script>

{#if compact}
	<span
		class="compact-metrics"
		aria-label="Resources for {device.name}"
		title={device.status !== 'online'
			? 'Resources unavailable while offline'
			: failed
				? 'Readings unavailable'
				: 'Live readings · refresh every 15 seconds'}
	>
		<span
			>CPU <strong
				>{device.status === 'online' && metrics && !failed
					? `${metrics.cpu_percent.toFixed(0)}%`
					: '—'}</strong
			></span
		>
		<span
			>RAM <strong
				>{device.status === 'online' && metrics && !failed
					? `${memoryPercent.toFixed(0)}%`
					: '—'}</strong
			></span
		>
		<span
			>Disk <strong
				>{device.status === 'online' && rootDisk && !failed
					? `${diskPercent.toFixed(0)}%`
					: '—'}</strong
			></span
		>
	</span>
{:else}
	<section
		class="host-metrics"
		class:is-stale={device.status !== 'online' || failed}
		aria-label="Resource readings for {device.name}"
	>
		<div class="metrics-heading">
			<span>Resources</span>
			<span
				>{device.status !== 'online'
					? metrics
						? 'Last reading · offline'
						: 'Unavailable while offline'
					: failed
						? 'Readings unavailable'
						: metrics
							? 'Refreshes every 15s'
							: 'Reading resources…'}</span
			>
		</div>
		<div class="metrics-grid">
			<div class="metric">
				<span class="metric-label">CPU</span>
				<strong>{metrics ? `${metrics.cpu_percent.toFixed(1)}%` : '—'}</strong>
				<div class="metric-track"><span style:width="{metrics?.cpu_percent ?? 0}%"></span></div>
			</div>
			<div class="metric">
				<span class="metric-label">Memory</span>
				<strong>{metrics ? `${memoryPercent.toFixed(0)}%` : '—'}</strong>
				<div class="metric-track"><span style:width="{memoryPercent}%"></span></div>
				<small
					>{metrics
						? `${size(metrics.memory_used)} / ${size(metrics.memory_total)}`
						: 'No reading'}</small
				>
			</div>
			<div class="metric">
				<span class="metric-label">Disk {rootDisk?.mount ?? '/'}</span>
				<strong>{rootDisk ? `${diskPercent.toFixed(0)}%` : '—'}</strong>
				<div class="metric-track"><span style:width="{diskPercent}%"></span></div>
				<small>{rootDisk ? `${size(rootDisk.used)} / ${size(rootDisk.total)}` : 'No reading'}</small
				>
			</div>
		</div>
		{#if extraDisks.length}
			<details class="metrics-disks">
				<summary
					>{extraDisks.length} additional {extraDisks.length === 1
						? 'filesystem'
						: 'filesystems'}</summary
				>
				{#each extraDisks as disk (disk.mount)}
					<div><span>{disk.mount}</span><span>{size(disk.used)} / {size(disk.total)}</span></div>
				{/each}
			</details>
		{/if}
		{#if metrics}<div class="metrics-time">
				Read at {new Date(metrics.collected_at).toLocaleTimeString('en-GB', {
					timeZone: 'Europe/Budapest',
					hour: '2-digit',
					minute: '2-digit',
					second: '2-digit'
				})}
			</div>{/if}
	</section>
{/if}
