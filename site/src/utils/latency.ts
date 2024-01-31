import type { Theme } from "@emotion/react";

export const getLatencyColor = (theme: Theme, latency?: number) => {
  if (!latency) {
    return theme.palette.text.secondary;
  }

  let color = theme.roles.success.fill.solid;

  if (latency >= 150 && latency < 300) {
    color = theme.roles.warning.fill.solid;
  } else if (latency >= 300) {
    color = theme.roles.error.fill.solid;
  }
  return color;
};
