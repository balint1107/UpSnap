/** Build a readable physical-value scale without clipping observations.
 * @param values Watts for power, kWh for energy; invalid/negative values are ignored.
 * @param energy Keep a zero bar baseline when true; otherwise fit observed power with padding.
 * @returns Finite min/max bounds and evenly spaced ticks in the input units, including empty/flat data.
 */
export function powerScale(values: number[], energy = false) {
	const valid = values.filter((v) => Number.isFinite(v) && v >= 0);
	const low = Math.min(...valid, valid.length ? Infinity : 0);
	const high = Math.max(...valid, 0);
	const span = Math.max(high - low, energy ? 0.005 : 10);
	const bottom = energy ? 0 : Math.max(0, low - span * 0.12);
	const top = high + span * 0.12;
	const rough = Math.max(top - bottom, energy ? 0.005 : 10) / (energy ? 4 : 8);
	const magnitude = 10 ** Math.floor(Math.log10(rough));
	const step = [1, 2, 5, 10].map((n) => n * magnitude).find((n) => n >= rough) ?? magnitude * 10;
	const min = energy ? 0 : Math.floor(bottom / step) * step;
	const max = Math.max(min + step, Math.ceil(top / step) * step);
	const ticks = Array.from(
		{ length: Math.round((max - min) / step) + 1 },
		(_, i) => min + i * step
	);
	return { min, max, ticks };
}

/** Select and sort observations inside the displayed history window before scaling or drawing.
 * @param points Samples with Unix-second t and nonnegative watts/kWh value; invalid samples are ignored.
 * @param kind Power uses a trailing 24h window; energy retains 30 days around the latest calendar total.
 * @param now Current Unix seconds for power's right edge (injectable for deterministic tests).
 * @returns Visible points and finite start/end Unix seconds; sparse power history keeps a minimum 5min span.
 */
export function powerHistory(
	points: { t: number; value: number }[],
	kind: 'power' | 'energy' = 'power',
	now = Date.now() / 1000
) {
	const valid = points.filter((p) => Number.isFinite(p.t) && Number.isFinite(p.value) && p.value >= 0);
	const end = kind === 'power' ? now : Math.max(...valid.map((p) => p.t), valid.length ? -Infinity : now) + 43200;
	const cutoff = end - (kind === 'power' ? 86400 : 30 * 86400);
	const visible = valid.filter((p) => p.t >= cutoff && p.t <= end).sort((a, b) => a.t - b.t);
	const start = kind === 'power'
		? Math.max(cutoff, Math.min(end - 300, ...visible.map((p) => p.t)))
		: Math.max(cutoff, visible.length ? visible[0].t - 43200 : end - 86400);
	return { points: visible, start, end };
}
