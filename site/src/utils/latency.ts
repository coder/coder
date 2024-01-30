import type { Theme } from "@emotion/react";

export const getLatencyColor = (theme: Theme, latency?: number) => {
  if (!latency) {
    return theme.palette.text.secondary;
  }

  let color = theme.experimental.roles.success.fill.solid;

  if (latency >= 150 && latency < 300) {
    color = theme.experimental.roles.warning.fill.solid;
  } else if (latency >= 300) {
    color = theme.experimental.roles.error.fill.solid;
  }
  return color;
};
