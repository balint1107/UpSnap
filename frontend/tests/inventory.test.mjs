import assert from 'node:assert/strict';
import test from 'node:test';
import { reconcileInventory } from '../src/lib/helpers/inventory.ts';

const definition = {
	id: 'router:test', driver_id: 'test', driver: 'router', category: 'networking',
	name: 'Router', address: '192.0.2.1', capabilities: ['wifi'],
	permissions: { read: true, control: true }, state: 'unknown', checked_at: null
};

test('cold inventory creates known rows without a diagnostic payload', () => {
	const [device] = reconcileInventory([], [definition]);
	assert.equal(device.id, definition.id);
	assert.equal(device.router, undefined);
	assert.equal(device.state, 'unknown');
	assert.equal(device.failed, false);
});

test('metadata refresh preserves newer action readings but immediately replaces permissions', () => {
	const reading = { bands: [{ id: '0', on: false }] };
	const old = { ...definition, checked_at: '2026-10-31T12:01:00Z', state: 'online', router: reading, refreshing: true, failed: false };
	const [device] = reconcileInventory([old], [{ ...definition, name: 'Renamed', permissions: { read: true, control: false }, checked_at: '2026-10-31T12:00:00Z' }]);
	assert.equal(device.router, reading);
	assert.equal(device.name, 'Renamed');
	assert.equal(device.state, 'online');
	assert.equal(device.refreshing, true);
	assert.equal(device.permissions.control, false);
	assert.equal(old.name, 'Router');
});

test('permission revocation removes rows instead of keeping cached devices', () => {
	assert.deepEqual(reconcileInventory([{ ...definition, refreshing: false, failed: false }], []), []);
});
