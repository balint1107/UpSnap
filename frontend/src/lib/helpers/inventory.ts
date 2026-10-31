import type { DashboardDevice, DeviceEnvelope } from '../types/inventory.ts';

/** Replace definitions with authorized entries, keeping driver data and newer observations by stable ID.
 * Previous entries absent from definitions are removed. Incoming permissions always win.
 * Inputs are borrowed; the returned array and entries are new objects.
 */
export function reconcileInventory(
	previous: DashboardDevice[],
	definitions: DeviceEnvelope[]
): DashboardDevice[] {
	const old = new Map(previous.map((device) => [device.id, device]));
	return definitions.map((definition) => {
		const prior = old.get(definition.id);
		const newer =
			prior?.checked_at &&
			(!definition.checked_at || Date.parse(prior.checked_at) > Date.parse(definition.checked_at));
		return {
			...prior,
			...definition,
			...(newer ? { state: prior.state, checked_at: prior.checked_at } : {}),
			refreshing: prior?.refreshing ?? false,
			failed: prior?.failed ?? false
		};
	});
}
