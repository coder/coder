import HelpOutline from "@mui/icons-material/HelpOutline";
import Tooltip from "@mui/material/Tooltip";
import CircularProgress from "@mui/material/CircularProgress";
import { visuallyHidden } from "@mui/utils";
import { useTheme } from "@emotion/react";
import { type FC } from "react";
import { getLatencyColor } from "utils/latency";
import { Abbr } from "components/Abbr/Abbr";

interface ProxyStatusLatencyProps {
  latency?: number;
  isLoading?: boolean;
}

export const ProxyStatusLatency: FC<ProxyStatusLatencyProps> = ({
  latency,
  isLoading,
}) => {
  const theme = useTheme();
  // Always use the no latency color for loading.
  const color = getLatencyColor(theme, isLoading ? undefined : latency);

  if (isLoading) {
    return (
      <Tooltip title="Loading latency...">
        <CircularProgress
          size={14}
          css={{ marginLeft: "auto" }}
          style={{ color }}
        />
      </Tooltip>
    );
  }

  if (!latency) {
    const notAvailableText = "Latency not available";
    return (
      <Tooltip title={notAvailableText}>
        <>
          <span css={{ ...visuallyHidden }}>{notAvailableText}</span>

          <HelpOutline
            css={{
              marginLeft: "auto",
              fontSize: "14px !important",
            }}
            style={{ color }}
          />
        </>
      </Tooltip>
    );
  }

  return (
    <p css={{ fontSize: 13, margin: "0 0 0 auto" }} style={{ color }}>
      <span css={{ ...visuallyHidden }}>Latency: </span>
      {latency.toFixed(0)}
      <Abbr title="milliseconds">ms</Abbr>
    </p>
  );
};
