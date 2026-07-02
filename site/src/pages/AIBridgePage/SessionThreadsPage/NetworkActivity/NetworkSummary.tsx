import { CheckIcon, TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "#/utils/cn";
import type { NetworkActivity } from "./types";
import { computeNetworkCounts } from "./types";

interface NetworkSummaryProps {
	networkActivity?: NetworkActivity;
}

/**
 * Network row rendered inside the Session summary side panel.
 *
 * States:
 * - No events at all -> "No activity".
 * - One or more counts -> a line per non-zero count, each with its severity
 *   icon. Multi-line layouts stack right-aligned.
 */
export const NetworkSummary: FC<NetworkSummaryProps> = ({
	networkActivity,
}) => {
	const counts = computeNetworkCounts(networkActivity);
	const isEmpty = counts.total === 0;

	return (
		<div
			className={cn(
				"flex justify-between",
				isEmpty ? "items-center" : "items-start",
			)}
		>
			<dt
				className={cn(
					"shrink-0 font-normal whitespace-nowrap",
					!isEmpty && "pt-px",
				)}
			>
				Network
			</dt>
			<dd className="ml-4 min-w-0 text-content-primary flex flex-col items-end gap-1">
				{isEmpty ? (
					<span className="text-content-secondary">No activity</span>
				) : (
					<>
						{counts.allowed > 0 && (
							<NetworkSummaryLine
								label={`${counts.allowed} allowed`}
								tone="success"
							/>
						)}
						{counts.warnings > 0 && (
							<NetworkSummaryLine
								label={`${counts.warnings} ${counts.warnings === 1 ? "warning" : "warnings"}`}
								tone="warning"
							/>
						)}
						{counts.errors > 0 && (
							<NetworkSummaryLine
								label={`${counts.errors} ${counts.errors === 1 ? "error" : "errors"}`}
								tone="error"
							/>
						)}
					</>
				)}
			</dd>
		</div>
	);
};

type Tone = "success" | "warning" | "error";

interface NetworkSummaryLineProps {
	label: string;
	tone: Tone;
}

const NetworkSummaryLine: FC<NetworkSummaryLineProps> = ({ label, tone }) => (
	<span className="inline-flex items-center gap-1.5 text-sm font-mono text-content-primary">
		{label}
		<NetworkToneIcon tone={tone} />
	</span>
);

const NetworkToneIcon: FC<{ tone: Tone }> = ({ tone }) => {
	switch (tone) {
		case "success":
			return (
				<CheckIcon className="size-icon-xs p-0.5 text-content-success shrink-0" />
			);
		case "warning":
			return (
				<TriangleAlertIcon className="size-icon-xs p-0.5 text-content-warning shrink-0" />
			);
		case "error":
			return (
				<TriangleAlertIcon className="size-icon-xs p-0.5 text-content-destructive shrink-0" />
			);
	}
};
