import assert from 'node:assert/strict';
import test from 'node:test';
import { readPowerResponse } from '../src/lib/helpers/power.ts';

for (const status of ['offline', 'online']) {
	test(`HTTP 200 preserves the returned device state: ${status}`, async () => {
		const record = { id: 'backup', status };
		assert.deepEqual(await readPowerResponse(Response.json(record)), record);
	});
}

test('HTTP failure reports the server error instead of pretending to change state', async () => {
	await assert.rejects(
		readPowerResponse(Response.json({ message: 'Suspend permission denied' }, { status: 400 })),
		/Suspend permission denied/
	);
});
