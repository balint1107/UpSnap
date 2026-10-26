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
