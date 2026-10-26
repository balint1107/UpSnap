import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import { powerScale } from '../src/lib/utils/powerScale.ts';

test('power axis reveals small variations without clipping extrema', () => {
 const s = powerScale([235.2, 239.8, 237.1]);
 assert.ok(s.min > 200 && s.min <= 235.2);
 assert.ok(s.max >= 239.8 && s.max < 260);
 assert.ok(s.ticks.length >= 3);
});
test('energy bars keep a zero baseline and distinguish Wh increments', () => {
 const s = powerScale([0.119, 0.123], true);
 assert.equal(s.min, 0); assert.ok(s.max >= .123);
 assert.equal(new Set(s.ticks.map(v => v.toFixed(3))).size, s.ticks.length);
});
test('zero, flat and invalid readings produce finite nondegenerate scales', () => {
 for (const values of [[], [0], [240,240], [NaN,Infinity,-1]]) {
  const s = powerScale(values); assert.ok(Number.isFinite(s.min)); assert.ok(s.max > s.min);
 }
});

test('wide power ranges use finer intervals without excessive headroom', () => {
 const s = powerScale([90, 580]);
 assert.ok(s.ticks[1] - s.ticks[0] <= 100);
 assert.ok(s.max <= 700);
});
