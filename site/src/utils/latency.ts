export const getLatencyColor = (
	latency?: number,
	text: "text" | "background" = "text",
) => {
	if (!latency) {
		return "text-content-secondary";
	}

	let color = text === "text" ? "text-content-success" : "bg-surface-success";

	if (latency >= 150 && latency < 300) {
		color = text === "text" ? "text-content-warning" : "bg-surface-warning";
	} else if (latency >= 300) {
		color = text === "text" ? "text-content-error" : "bg-surface-error";
	}
	return color;
};
