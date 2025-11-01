import { type Theme, useTheme } from "@emotion/react";
import type { DERPRegion, WorkspaceAgent } from "api/typesGenerated";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import type { FC } from "react";
import { getLatencyColor } from "utils/latency";

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
			<HelpTooltipTrigger asChild>
				<span
					role="presentation"
					aria-label="latency"
					css={{ cursor: "pointer", color: latency.color }}
				>
					{Math.round(latency.latency_ms)}ms
				</span>
			</HelpTooltipTrigger>
			<HelpTooltipContent>
				<HelpTooltipTitle>Latency</HelpTooltipTitle>
				<HelpTooltipText>
					This is the latency overhead on non peer to peer connections. The
					first row is the preferred relay.
				</HelpTooltipText>
				<div className="mt-4 flex flex-col gap-2">
					{Object.entries(agent.latency)
						.sort(([, a], [, b]) => a.latency_ms - b.latency_ms)
						.map(([regionName, region]) => (
							<div
								key={regionName}
								className="flex flex-row justify-between gap-1"
								css={
									region.preferred && {
										color: theme.palette.text.primary,
									}
								}
							>
								<strong>{regionName}</strong>
								{Math.round(region.latency_ms)}ms
							</div>
						))}
				</div>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};
