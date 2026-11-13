const latencyColors = {
	good: {
		text: "text-content-success",
		icon: "text-content-success",
		background: "bg-content-success",
	},
	warning: {
		text: "text-content-warning",
		icon: "text-border-warning",
		background: "bg-border-warning",
	},
	critical: {
		text: "text-content-destructive",
		icon: "text-content-destructive",
		background: "bg-content-destructive",
	},
	unknown: {
		text: "text-content-secondary",
		icon: "text-content-secondary",
		background: "bg-content-secondary",
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
	type: "text" | "icon" | "background" = "text",
) => latencyColors[getLatencyLevel(latency)][type];
