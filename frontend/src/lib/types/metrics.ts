export type HostMetrics = {
	available: boolean;
	cpu_percent: number;
	memory_used: number;
	memory_total: number;
	disks: { mount: string; used: number; total: number }[];
	collected_at: string;
};
