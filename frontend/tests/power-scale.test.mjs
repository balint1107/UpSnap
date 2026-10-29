import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import { powerHistory, powerScale } from '../src/lib/utils/powerScale.ts';

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


test('expired power samples cannot reach beyond the 24-hour plot or distort its scale', () => {
 const now = 200000;
 const history = powerHistory([{t: now - 90000, value: 9999}, {t: now - 86400, value: 200}, {t: now - 60, value: 210}, {t: now + 60, value: 4000}], 'power', now);
 assert.equal(history.start, now - 86400);
 assert.deepEqual(history.points.map(p => p.value), [200, 210]);
 assert.ok(powerScale(history.points.map(p => p.value)).max < 300);
 for (const p of history.points) {
  const x = (p.t - history.start) / (history.end - history.start);
  assert.ok(x >= 0 && x <= 1);
 }
});
test('sparse, empty, unsorted and invalid histories keep finite bounds without invented observations', () => {
 const now = 200000;
 const input = [{t: now, value: 10}, {t: now - 60, value: 20}, {t: NaN, value: 1}, {t: now, value: -1}];
 const h = powerHistory(input, 'power', now);
 assert.deepEqual(h.points.map(p => p.t), [now - 60, now]);
 assert.equal(input[0].t, now);
 assert.equal(h.end - h.start, 300);
 assert.equal(powerHistory([{t: now - 90000, value: 10}], 'power', now).points.length, 0);
 for (const kind of ['power', 'energy']) {
  const empty = powerHistory([], kind, now);
  assert.ok(Number.isFinite(empty.start) && empty.end > empty.start);
 }
});
test('expired energy samples are excluded from bars and tick candidates', () => {
 const end = 50 * 86400;
 const h = powerHistory([{t: end - 40 * 86400, value: 999}, {t: end - 86400, value: 1}, {t: end, value: 2}], 'energy', end);
 assert.deepEqual(h.points.map(p => p.value), [1, 2]);
 assert.ok(h.points.every(p => p.t >= h.start && p.t <= h.end));
});
