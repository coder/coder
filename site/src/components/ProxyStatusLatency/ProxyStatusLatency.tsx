import { useTheme } from "@emotion/react";
import HelpOutline from "@mui/icons-material/HelpOutline";
import Tooltip from "@mui/material/Tooltip";
import { type FC } from "react";
import { getLatencyColor } from "utils/latency";
import CircularProgress from "@mui/material/CircularProgress";
import { visuallyHidden } from "@mui/utils";
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
    const notAvailableText = "Latency not available";
    return (
      <Tooltip title={notAvailableText}>
        <>
          <span css={{ ...visuallyHidden }}>{notAvailableText}</span>

          <HelpOutline
            css={{
              marginLeft: "auto",
              fontSize: "14px !important",
              color,
            }}
          />
        </>
      </Tooltip>
    );
  }

  return (
    <p css={{ color, fontSize: 13, margin: "0 0 0 auto" }}>
      <span css={{ ...visuallyHidden }}>Latency: </span>
      {latency.toFixed(0)}
      <Abbr title="milliseconds">ms</Abbr>
    </p>
  );
};
