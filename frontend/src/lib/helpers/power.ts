import type { Device } from '$lib/types/device';

// A successful power response is a device record. Its status is a device state,
// while HTTP success belongs to the Response object.
export async function readPowerResponse(response: Response): Promise<Device> {
	const data = await response.json();
	if (!response.ok) {
		throw new Error(data.message || `Power action failed (HTTP ${response.status})`);
	}
	return data as Device;
}
