import { useTheme } from "@emotion/react";
import HelpOutline from "@mui/icons-material/HelpOutline";
import Tooltip from "@mui/material/Tooltip";
import { type FC } from "react";
import { getLatencyColor } from "utils/latency";
import CircularProgress from "@mui/material/CircularProgress";

interface ProxyStatusLatencyProps {
  latency?: number;
  isLoading?: boolean;
}

export const ProxyStatusLatency: FC<ProxyStatusLatencyProps> = ({
  latency,
  isLoading,
}) => {
  const theme = useTheme();
  const color = getLatencyColor(theme, latency);

  if (isLoading) {
    return (
      <Tooltip title="Loading latency...">
        <CircularProgress
          size={14}
          css={{
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
          css={{
            marginLeft: "auto",
            fontSize: "14px !important",
            color,
          }}
        />
      </Tooltip>
    );
  }

  return (
    <div css={{ color, fontSize: 13, marginLeft: "auto" }}>
      {latency.toFixed(0)}ms
    </div>
  );
};
