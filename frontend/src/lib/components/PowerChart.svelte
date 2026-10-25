<script lang="ts">
	let {
		points,
		kind = 'power'
	}: { points: { t: number; value: number }[]; kind?: 'power' | 'energy' } = $props();
	const valid = $derived(
		points.filter((p) => Number.isFinite(p.t) && Number.isFinite(p.value) && p.value >= 0)
	);
	const end = $derived(Math.max(Date.now() / 1000, ...valid.map((p) => p.t)));
	const start = $derived(end - (kind === 'power' ? 86400 : 30 * 86400));
	const maximum = $derived(
		Math.max(kind === 'power' ? 1 : 0.001, ...valid.map((p) => p.value)) * 1.15
	);
	const unit = $derived(kind === 'power' ? 'W' : 'kWh');
	/** Map Unix seconds t into the fixed 24h/30d plot; return SVG x coordinate. */
	function x(t: number) {
		return 42 + ((t - start) / (end - start)) * 342;
	}
	/** Map nonnegative physical value into the plot; return SVG y coordinate. */
	function y(value: number) {
		return 165 - (value / maximum) * 145;
	}
	/** Format Unix seconds t in the plug's Budapest calendar for readable labels/tooltips. */
	function label(t: number) {
		return new Date(t * 1000).toLocaleString('en-GB', {
			timeZone: 'Europe/Budapest',
			...(kind === 'power'
				? { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }
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

<div class="chart">
	<h3>{kind === 'power' ? 'Power · last 24 hours' : 'Daily energy · last 30 days'}</h3>
	{#if valid.length}
		<svg
			viewBox="0 0 400 210"
			role="img"
			aria-label={kind === 'power'
				? 'Recorded power in watts over the last 24 hours'
				: 'Observed daily energy totals in kilowatt-hours'}
		>
			{#each [0, 0.5, 1] as step}
				<line x1="42" x2="384" y1={y(maximum * step)} y2={y(maximum * step)} class="grid" />
				<text x="35" y={y(maximum * step) + 4} text-anchor="end"
					>{(maximum * step).toFixed(kind === 'power' ? 0 : 3)}</text
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
						height={165 - y(p.value)}
						rx="2"
						class="point"><title>{label(p.t)} · {p.value.toFixed(3)} kWh</title></rect
					>{/each}
			{/if}
			<text x="42" y="190">{label(start)}</text><text x="384" y="190" text-anchor="end"
				>{label(end)}</text
			><text x="42" y="12">{unit}</text>
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
		max-height: 220px;
		overflow: visible;
	}
	text {
		fill: var(--monitor-muted);
		font-size: 14px;
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
