import type { Theme } from "@emotion/react";
import type { HealthSeverity } from "api/typesGenerated";

export const healthyColor = (theme: Theme, severity: HealthSeverity) => {
	switch (severity) {
		case "ok":
			return theme.roles.success.fill.solid;
		case "warning":
			return theme.roles.warning.fill.solid;
		case "error":
			return theme.roles.error.fill.solid;
	}
};
