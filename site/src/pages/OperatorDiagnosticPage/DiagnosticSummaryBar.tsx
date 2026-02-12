import { Stats, StatsItem } from "components/Stats/Stats";
import { StatusIndicatorDot } from "components/StatusIndicator/StatusIndicator";
import type { FC } from "react";
import { cn } from "utils/cn";
import type { DiagnosticSummary, DiagnosticTimeWindow } from "./types";

interface DiagnosticSummaryBarProps {
	summary: DiagnosticSummary;
	timeWindow: DiagnosticTimeWindow;
}

function latencyColor(ms: number | null): string {
	if (ms === null) return "text-content-secondary";
	if (ms < 50) return "text-content-success";
	if (ms < 200) return "text-content-warning";
	return "text-content-destructive";
}

export const DiagnosticSummaryBar: FC<DiagnosticSummaryBarProps> = ({
	summary,
}) => {
	const lostCount = summary.by_status.lost;
	const dominantMode =
		summary.network.p2p_connections >= summary.network.derp_connections
			? "P2P"
			: "DERP";

	return (
		<Stats>
			<StatsItem label="Sessions" value={summary.total_sessions} />
			<StatsItem
				label="Active"
				value={
					<span className="inline-flex items-center gap-1.5">
						<StatusIndicatorDot variant="success" size="sm" />
						{summary.active_connections}
					</span>
				}
			/>
			{lostCount > 0 && (
				<StatsItem
					label="Lost"
					value={
						<span className="inline-flex items-center gap-1.5">
							<StatusIndicatorDot variant="warning" size="sm" />
							{lostCount}
						</span>
					}
				/>
			)}
			<StatsItem
				label="Avg Latency"
				value={
					<span className={cn(latencyColor(summary.network.avg_latency_ms))}>
						{summary.network.avg_latency_ms !== null
							? `${summary.network.avg_latency_ms.toFixed(0)}ms`
							: "N/A"}
					</span>
				}
			/>
			<StatsItem label="Network" value={dominantMode} />
		</Stats>
	);
};
