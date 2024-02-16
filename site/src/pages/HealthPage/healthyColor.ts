import type { Theme } from "@emotion/react";
import type { HealthSeverity } from "api/typesGenerated";

export const healthyColor = (theme: Theme, severity: HealthSeverity) => {
  switch (severity) {
    case "ok":
      return theme.palette.success.light;
    case "warning":
      return theme.palette.warning.light;
    case "error":
      return theme.palette.error.light;
  }
};
