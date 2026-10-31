<script lang="ts">
	import type { DashboardDevice } from '$lib/types/inventory';
	let { device, failed = false }: { device: DashboardDevice; failed?: boolean } = $props();
</script>

<div
	class="server-card device-row bg-base-100"
	data-inventory-id={device.id}
	data-category={device.category}
	data-state={device.state}
	aria-busy={!failed}
>
	<div class="device-row-summary">
		<span class="row-host-name">{device.name}</span>
		<span class="row-host-ip">{device.address}</span>
		<span class="host-state">{failed ? 'Details unavailable' : 'Checking…'}</span>
		<span class="compact-metrics">—</span>
	</div>
</div>
