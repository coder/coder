import { Theme } from "@mui/material/styles";

export const getLatencyColor = (theme: Theme, latency?: number) => {
  if (!latency) {
    return theme.palette.text.secondary;
  }

  let color = theme.palette.success.light;

  if (latency >= 150 && latency < 300) {
    color = theme.palette.warning.light;
  } else if (latency >= 300) {
    color = theme.palette.error.light;
  }
  return color;
};
