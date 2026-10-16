<script lang="ts">
	import { serviceStatus, refreshServices } from '$lib/stores/services';
	let { hostIp, embedded = false }: { hostIp?: string; embedded?: boolean } = $props();
	const services = $derived($serviceStatus.services.filter((s) => !hostIp || s.host_ip === hostIp));
	const groups = $derived(
		Array.from(new Set(services.map((s) => s.host_ip))).map((ip) => ({
			ip,
			name: services.find((s) => s.host_ip === ip)?.host_name ?? ip,
			services: services.filter((s) => s.host_ip === ip)
		}))
	);
	const labels: Record<string, string> = {
		up: 'Available',
		down: 'Unavailable',
		offline: 'Host offline',
		disabled: 'Disabled',
		degraded: 'Degraded',
		unknown: 'Unknown',
		stale: 'Stale'
	};
	function webUrl(url?: string) {
		return url && /^https?:\/\//i.test(url) ? url : undefined;
	}
</script>

<section
	class="services-section"
	class:embedded
	aria-label={embedded ? 'Host services' : 'Service availability'}
>
	<div class="services-heading">
		<div>
			{#if !embedded}<div class="monitor-eyebrow">Application availability</div>{/if}
			<h2>Services</h2>
			<p>Checked from server-rpi every 30 seconds, including while this dashboard is closed.</p>
		</div>
		<button class="btn btn-sm" disabled={$serviceStatus.fetching} onclick={refreshServices}
			>Refresh services</button
		>
	</div>
	{#if $serviceStatus.failed}<p class="service-error" role="status">
			Cannot refresh service checks. Previous results are stale.
		</p>{/if}
	{#each groups as group (group.ip)}
		{#if !embedded}<h3 class="service-group-heading">{group.name} <span>{group.ip}</span></h3>{/if}
		<div class="service-grid">
			{#each group.services as service (service.id)}
				<article
					class="service-panel"
					data-service={service.id}
					data-service-state={$serviceStatus.failed ? 'stale' : service.status}
				>
					<div class="service-heading">
						<h3>
							{#if webUrl(service.url)}<a
									href={webUrl(service.url)}
									target="_blank"
									rel="noopener noreferrer">{service.name}</a
								>{:else}{service.name}{/if}
						</h3>
						<span class="service-state"
							>{$serviceStatus.failed ? 'Stale' : labels[service.status] || 'Unknown'}</span
						>
					</div>
					<p class="service-detail">{service.detail}</p>
					<div class="service-foot">
						Checked {new Date(service.checked_at).toLocaleTimeString('en-GB', {
							timeZone: 'Europe/Budapest'
						})} · {service.latency_ms} ms
					</div>
				</article>
			{/each}
		</div>
	{:else}
		<p>
			{$serviceStatus.fetching
				? 'Loading service checks…'
				: $serviceStatus.failed
					? 'Pi collector unavailable.'
					: 'No service checks configured for this host.'}
		</p>
	{/each}

	{#if services.length && !embedded}<p class="services-note">
			Results are marked stale if the Pi collector stops reporting for 90 seconds. HTTP availability
			does not verify every application function.
		</p>{/if}
</section>

<style>
	.services-section.embedded {
		margin-top: 0;
		padding-top: 1.2rem;
	}
	.embedded .services-heading p {
		display: none;
	}
	.embedded h2 {
		font-size: 1rem;
	}
	.service-group-heading {
		margin-block: 1rem 0.5rem;
		font-size: 1rem;
	}
	.service-group-heading span {
		margin-left: 0.5rem;
		color: var(--monitor-muted);
		font-size: 0.8rem;
		font-weight: 400;
	}
	.services-section {
		margin-top: 2.5rem;
		border-top: 1px solid var(--monitor-border);
		padding-top: 2rem;
	}
	.services-heading,
	.service-heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}
	.services-heading {
		margin-bottom: 1.2rem;
	}
	h2 {
		font-size: 1.5rem;
		font-weight: 650;
		margin-block: 0.3rem;
	}
	.services-heading p,
	.services-note,
	.service-foot {
		color: var(--monitor-muted);
		font-size: 0.8rem;
	}
	.service-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 0.65rem;
	}
	.service-panel {
		display: flex;
		flex-direction: column;
		height: 9rem;
		min-width: 0;
		overflow: auto;
		background: var(--color-base-100);
		border: 1px solid var(--monitor-border);
		border-radius: 8px;
		padding: 0.8rem;
	}
	.service-foot {
		margin-top: auto;
		padding-top: 0.2rem;
	}
	h3 {
		font-weight: 650;
	}
	h3 a:hover {
		text-decoration: underline;
	}
	.service-state {
		font-size: 0.75rem;
		color: var(--monitor-muted);
		white-space: nowrap;
	}
	[data-service-state='up'] .service-state {
		color: var(--color-success);
	}
	[data-service-state='down'] .service-state {
		color: var(--color-error);
	}
	[data-service-state='degraded'] .service-state,
	[data-service-state='stale'] .service-state {
		color: var(--color-warning);
	}
	.service-detail {
		flex: 0 0 auto;
		font-size: 0.8rem;
		margin-block: 0.35rem;
		min-height: 0;
	}
	.services-note {
		margin-top: 1rem;
	}
	.service-error {
		color: var(--color-warning);
		margin-bottom: 1rem;
	}
	@media (max-width: 600px) {
		.services-heading {
			align-items: flex-start;
			flex-direction: column;
		}
		.service-grid {
			grid-template-columns: minmax(0, 1fr);
		}
		.service-heading {
			gap: 0.5rem;
		}
	}
</style>
