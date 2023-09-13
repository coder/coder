import { useTheme } from "@mui/material/styles";
import HelpOutline from "@mui/icons-material/HelpOutline";
import Box from "@mui/material/Box";
import Tooltip from "@mui/material/Tooltip";
import { FC } from "react";
import { getLatencyColor } from "utils/latency";
import CircularProgress from "@mui/material/CircularProgress";

export const ProxyStatusLatency: FC<{
  latency?: number;
  isLoading?: boolean;
}> = ({ latency, isLoading }) => {
  const theme = useTheme();
  const color = getLatencyColor(theme, latency);

  if (isLoading) {
    return (
      <Tooltip title="Loading latency...">
        <CircularProgress
          size={14}
          sx={{
            // Always use the no latency color for loading.
            color: getLatencyColor(theme, undefined),
            marginLeft: "auto",
          }}
        />
      </Tooltip>
    );
  }

  if (!latency) {
    return (
      <Tooltip title="Latency not available">
        <HelpOutline
          sx={{
            ml: "auto",
            fontSize: "14px !important",
            color,
          }}
        />
      </Tooltip>
    );
  }

  return (
    <Box sx={{ color, fontSize: 13, marginLeft: "auto" }}>
      {latency.toFixed(0)}ms
    </Box>
  );
};
