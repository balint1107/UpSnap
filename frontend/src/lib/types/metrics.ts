export type HostMetrics = {
	available: boolean;
	cpu_percent: number;
	memory_used: number;
	memory_total: number;
	disks: { mount: string; used: number; total: number }[];
	gpus?: {
		id: string;
		name: string;
		source: string;
		utilization_percent: number | null;
		memory_used: number | null;
		memory_total: number | null;
		temperature_c: number | null;
		power_watts: number | null;
	}[];
	collected_at: string;
};
