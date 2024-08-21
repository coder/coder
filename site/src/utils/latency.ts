import type { Theme } from "@emotion/react";

export const getLatencyColor = (theme: Theme, latency?: number) => {
	if (!latency) {
		return theme.palette.text.secondary;
	}

	let color = theme.colorRoles.success.fill.solid;

	if (latency >= 150 && latency < 300) {
		color = theme.colorRoles.warning.fill.solid;
	} else if (latency >= 300) {
		color = theme.colorRoles.error.fill.solid;
	}
	return color;
};
