import { type Theme, useTheme } from "@emotion/react";
import { type FC } from "react";
import type { WorkspaceAgent, DERPRegion } from "api/typesGenerated";
import {
  HelpTooltipText,
  HelpTooltip,
  HelpTooltipTitle,
  HelpTooltipContent,
} from "components/HelpTooltip/HelpTooltip";
import { Stack } from "components/Stack/Stack";
import { getLatencyColor } from "utils/latency";
import { PopoverTrigger } from "components/Popover/Popover";

const getDisplayLatency = (theme: Theme, agent: WorkspaceAgent) => {
  // Find the right latency to display
  const latencyValues = Object.values(agent.latency ?? {});
  const latency =
    latencyValues.find((derp) => derp.preferred) ??
    // Accessing an array index can return undefined as well
    // for some reason TS does not handle that
    (latencyValues[0] as DERPRegion | undefined);

  if (!latency) {
    return undefined;
  }

  return {
    ...latency,
    color: getLatencyColor(theme, latency.latency_ms),
  };
};

interface AgentLatencyProps {
  agent: WorkspaceAgent;
}

export const AgentLatency: FC<AgentLatencyProps> = ({ agent }) => {
  const theme = useTheme();
  const latency = getDisplayLatency(theme, agent);

  if (!latency || !agent.latency) {
    return null;
  }

  return (
    <HelpTooltip>
      <PopoverTrigger>
        <span
          role="presentation"
          aria-label="latency"
          css={{ cursor: "pointer", color: latency.color }}
        >
          {Math.round(latency.latency_ms)}ms
        </span>
      </PopoverTrigger>
      <HelpTooltipContent>
        <HelpTooltipTitle>Latency</HelpTooltipTitle>
        <HelpTooltipText>
          This is the latency overhead on non peer to peer connections. The
          first row is the preferred relay.
        </HelpTooltipText>
        <HelpTooltipText>
          <Stack direction="column" spacing={1} css={{ marginTop: 16 }}>
            {Object.entries(agent.latency)
              .sort(([, a], [, b]) => a.latency_ms - b.latency_ms)
              .map(([regionName, region]) => (
                <Stack
                  direction="row"
                  key={regionName}
                  spacing={0.5}
                  justifyContent="space-between"
                  css={
                    region.preferred && {
                      color: theme.palette.text.primary,
                    }
                  }
                >
                  <strong>{regionName}</strong>
                  {Math.round(region.latency_ms)}ms
                </Stack>
              ))}
          </Stack>
        </HelpTooltipText>
      </HelpTooltipContent>
    </HelpTooltip>
  );
};
