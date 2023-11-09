import { dark } from "theme/mui";

// TODO: fix this, it'd be nice to make it "constant" but needs light mode awareness somehow

export const getLatencyColor = (latency?: number) => {
  if (!latency) {
    return dark.palette.text.secondary;
  }

  let color = dark.palette.success.light;

  if (latency >= 150 && latency < 300) {
    color = dark.palette.warning.light;
  } else if (latency >= 300) {
    color = dark.palette.error.light;
  }
  return color;
};
