const latencyColors = {
	good: { text: "text-content-success", background: "bg-content-green" },
	warning: {
		text: "text-content-warning",
		background: "bg-content-yellow",
	},
	critical: {
		text: "text-content-destructive",
		background: "bg-content-destructive",
	},
	unknown: {
		text: "text-content-secondary",
		background: "bg-surface-quaternary",
	},
} as const;

type LatencyLevel = keyof typeof latencyColors;

const getLatencyLevel = (latency?: number): LatencyLevel => {
	if (!latency) return "unknown";
	if (latency < 150) return "good";
	if (latency < 300) return "warning";
	return "critical";
};

export const getLatencyColor = (
	latency?: number,
	type: "text" | "background" = "text",
) => latencyColors[getLatencyLevel(latency)][type];
