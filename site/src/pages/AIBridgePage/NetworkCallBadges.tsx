import { BanIcon } from "lucide-react";
import type { FC } from "react";
import type { AIBridgeSessionNetworkCallSummary } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

interface NetworkCallBadgesProps {
	// summary is undefined when network call monitoring was not active for the
	// session, which renders as "Disabled".
	summary: AIBridgeSessionNetworkCallSummary | undefined;
}

export const NetworkCallBadges: FC<NetworkCallBadgesProps> = ({ summary }) => {
	if (!summary) {
		return (
			<span className="inline-flex items-center gap-1 whitespace-nowrap text-content-secondary">
				Disabled
				<InfoTooltip message="Network call monitoring was not active for this session." />
			</span>
		);
	}

	if (summary.total === 0) {
		return (
			<span className="whitespace-nowrap text-content-secondary">
				No activity
			</span>
		);
	}

	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<button
						type="button"
						aria-label="More info"
						className="flex items-center whitespace-nowrap border-0 bg-transparent p-0 text-inherit"
					>
						<Badge size="sm" className="rounded-e-none">
							{summary.total.toLocaleString("en-US")}
						</Badge>
						<Badge
							size="sm"
							svgSize="xs"
							className="gap-0 bg-surface-tertiary rounded-s-none text-content-warning"
						>
							<BanIcon className="flex-shrink-0" />
							{summary.blocked.toLocaleString("en-US")}
						</Badge>
					</button>
				</TooltipTrigger>
				<TooltipContent
					side="top"
					align="start"
					className="text-sm font-normal"
				>
					<div className="flex flex-col gap-1">
						<div className="flex items-center justify-between gap-4">
							<span className="text-content-secondary">Total calls</span>
							<span>{summary.total.toLocaleString("en-US")}</span>
						</div>
						<div className="flex items-center justify-between gap-4">
							<span className="text-content-secondary">Blocked</span>
							<span>{summary.blocked.toLocaleString("en-US")}</span>
						</div>
					</div>
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
