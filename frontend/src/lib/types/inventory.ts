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

export type Band = { id: string; name: string; on: boolean | null };
export type Router = {
	device: DeviceEnvelope;
	id: string;
	name: string;
	ip: string;
	model: string;
	status: string;
	bands: Band[];
	leds: boolean | null;
	led_control: boolean;
	led_scope: string;
	lcd: boolean;
	lcd_on: boolean | null;
	firmware: string;
	uptime: string;
	load: string[];
	cpu: string | null;
	memory_total: number | null;
	memory_free: number | null;
	checked_at: number;
};

export type Plug = {
	device: DeviceEnvelope;
	id: string;
	name: string;
	ip: string;
	status: string;
	detail: string;
	checked_at: string;
	reading: {
		power_w: number | null;
		energy_today_kwh: number | null;
		energy_month_kwh: number | null;
		voltage_v: number | null;
		current_a: number | null;
		relay_on: boolean | null;
		model: string;
		hardware: string;
		firmware: string;
	};
	history: { t: number; w: number }[];
	daily: { date: string; kwh: number }[];
};

/** Authorized device definition plus optional driver observations and independent request state. */
export type DashboardDevice = DeviceEnvelope & {
	router?: Router;
	appliance?: Plug;
	refreshing: boolean;
	failed: boolean;
};
