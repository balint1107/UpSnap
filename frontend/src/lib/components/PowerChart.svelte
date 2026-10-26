<script lang="ts">
	import { powerScale } from '$lib/utils/powerScale';
	let width = $state(600);
	const plotWidth = $derived(Math.max(260, width - 32));
	const left = 56;
	const right = $derived(plotWidth - 12);
	let {
		points,
		kind = 'power'
	}: { points: { t: number; value: number }[]; kind?: 'power' | 'energy' } = $props();
	const valid = $derived(
		points.filter((p) => Number.isFinite(p.t) && Number.isFinite(p.value) && p.value >= 0)
	);
	const end = $derived(
		kind === 'energy'
			? Math.max(0, ...valid.map((p) => p.t)) + 43200
			: Math.max(Date.now() / 1000, ...valid.map((p) => p.t))
	);
	const start = $derived(
		kind === 'power'
			? Math.max(end - 86400, Math.min(end - 300, ...valid.map((p) => p.t)))
			: Math.max(end - 30 * 86400, Math.min(...valid.map((p) => p.t)) - 43200)
	);
	const scale = $derived(
		powerScale(
			valid.map((p) => p.value),
			kind === 'energy'
		)
	);
	const times = $derived(
		kind === 'energy'
			? valid
					.filter(
						(_, i) =>
							i % Math.max(1, Math.ceil(valid.length / (plotWidth < 450 ? 3 : 5))) === 0 ||
							i === valid.length - 1
					)
					.map((p) => p.t)
			: Array.from(
					{ length: plotWidth < 450 ? 3 : 5 },
					(_, i) => start + ((end - start) * i) / (plotWidth < 450 ? 2 : 4)
				)
	);
	const unit = $derived(kind === 'power' ? 'W' : 'kWh');
	/** Map Unix seconds t into available history, bounded to 24h/30d; return SVG x coordinate. */
	function x(t: number) {
		return left + ((t - start) / (end - start)) * (right - left);
	}
	/** Map nonnegative physical value into the plot; return SVG y coordinate. */
	function y(value: number) {
		return 150 - ((value - scale.min) / (scale.max - scale.min)) * 130;
	}
	/** Format Unix seconds t in the plug's Budapest calendar for readable labels/tooltips. */
	function label(t: number) {
		return new Date(t * 1000).toLocaleString('en-GB', {
			timeZone: 'Europe/Budapest',
			...(kind === 'power'
				? { hour: '2-digit', minute: '2-digit' }
				: { day: '2-digit', month: 'short' })
		});
	}
	/** Draw sampled watts without connecting monitoring gaps longer than two minutes. */
	function line() {
		return valid
			.map((p, i) => `${i === 0 || p.t - valid[i - 1].t > 120 ? 'M' : 'L'}${x(p.t)},${y(p.value)}`)
			.join(' ');
	}
</script>

<div class="chart" bind:clientWidth={width}>
	<h3>{kind === 'power' ? 'Power · last 24 hours' : 'Daily energy · last 30 days'}</h3>
	{#if valid.length}
		<svg
			viewBox={`0 0 ${plotWidth} 190`}
			role="img"
			aria-label={kind === 'power'
				? 'Recorded power in watts over the last 24 hours'
				: 'Observed daily energy totals in kilowatt-hours'}
		>
			{#each scale.ticks as tick}
				<line x1={left} x2={right} y1={y(tick)} y2={y(tick)} class="grid" />
				<text x={left - 8} y={y(tick) + 4} text-anchor="end"
					>{tick.toFixed(kind === 'power' ? 1 : 3)}</text
				>
			{/each}
			{#if kind === 'power'}
				<path d={line()} class="trace" />
				{#each valid as p}<circle
						cx={x(p.t)}
						cy={y(p.value)}
						r={valid.length < 3 ? 3 : 1.5}
						class="point"><title>{label(p.t)} · {p.value.toFixed(1)} W</title></circle
					>{/each}
			{:else}
				{#each valid as p}<rect
						x={x(p.t) - 4}
						y={y(p.value)}
						width="8"
						height={150 - y(p.value)}
						rx="2"
						class="point"><title>{label(p.t)} · {p.value.toFixed(3)} kWh</title></rect
					>{/each}
			{/if}
			{#each times as time, i}
				<text
					x={x(time)}
					y="174"
					text-anchor={i === 0 ? 'start' : i === times.length - 1 ? 'end' : 'middle'}
					>{label(time)}</text
				>
			{/each}
			<text x={left} y="12">{unit}</text>
		</svg>
	{:else}<p>No recorded history yet.</p>{/if}
	{#if kind === 'energy'}<p>
			Observed daily totals; today is incomplete.
		</p>{:else if valid.length < 2}<p>Recording history every minute.</p>{/if}
</div>

<style>
	.chart {
		min-width: 0;
		padding: 1rem;
		border: 1px solid var(--monitor-border);
		border-radius: 8px;
	}
	h3 {
		font-size: 0.9rem;
		font-weight: 600;
		margin-bottom: 0.75rem;
	}
	svg {
		width: 100%;
		height: auto;

		overflow: visible;
	}
	text {
		fill: var(--monitor-muted);
		font-size: 11px;
	}
	.grid {
		stroke: var(--monitor-border);
		stroke-width: 1;
	}
	.trace {
		fill: none;
		stroke: #477aa8;
		stroke-width: 2;
	}
	.point {
		fill: #477aa8;
	}
	p {
		color: var(--monitor-muted);
		font-size: 0.75rem;
		margin-top: 0.5rem;
	}
</style>
