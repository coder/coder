import { Theme } from "@mui/material/styles";

export const healthyColor = (
  theme: Theme,
  healthy: boolean,
  hasWarnings?: boolean,
) => {
  if (healthy && !hasWarnings) {
    return theme.palette.success.light;
  }
  if (healthy && hasWarnings) {
    return theme.palette.warning.light;
  }
  return theme.palette.error.light;
};
