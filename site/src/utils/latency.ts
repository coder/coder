import type { Theme } from "@emotion/react";

export const getLatencyColor = (theme: Theme, latency?: number) => {
  if (!latency) {
    return theme.palette.text.secondary;
  }

  let color = theme.experimental.roles.success.outline;

  if (latency >= 150 && latency < 300) {
    color = theme.experimental.roles.warning.outline;
  } else if (latency >= 300) {
    color = theme.experimental.roles.error.outline;
  }
  return color;
};
