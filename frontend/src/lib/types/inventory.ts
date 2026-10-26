/** Common device/service availability; a powered-off relay is independent of availability. */
export type DeviceState =
	'online' | 'offline' | 'pending' | 'unknown' | 'stale' | 'degraded' | 'disabled';
export type DeviceCapability =
	| 'resources'
	| 'terminal'
	| 'wake'
	| 'suspend'
	| 'reboot'
	| 'boot'
	| 'wifi'
	| 'leds'
	| 'lcd'
	| 'power_metrics'
	| 'power_history';
export type DeviceCategory = 'computers' | 'networking' | 'home_appliances';
/** Stable metadata; driver_id targets existing APIs and checked_at is UTC ISO or null. */
export type DeviceEnvelope = {
	id: string;
	driver: 'computer' | 'router' | 'appliance';
	driver_id: string;
	category: DeviceCategory;
	name: string;
	address: string;
	collector_device_id?: string;
	state: DeviceState;
	checked_at: string | null;
	capabilities: DeviceCapability[];
	permissions: { read: boolean; control: boolean };
};
/** Shared category labels prevent driver-specific section names from diverging. */
export const deviceCategories: Record<DeviceCategory, string> = {
	computers: 'Computers',
	networking: 'Networking',
	home_appliances: 'Home appliances'
};
/** Return the stable inventory ID for recordID, independent of computer name/address. */
export function computerDeviceID(recordID: string): string {
	return `computer:${recordID}`;
}
