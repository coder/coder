import { Badge } from "components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { AIBridgeModelIcon } from "pages/AIBridgePage/RequestLogsPage/icons/AIBridgeModelIcon";
import type { FC } from "react";
import { cn } from "#/utils/cn";
import { formatDate } from "#/utils/time";
import { TokenBadges } from "../../TokenBadges";

interface PromptTableProps {
	timestamp: Date;
	model: string;
	inputTokens: number;
	outputTokens: number;
	tokenUsageMetadata?: Record<string, unknown>;
	className?: string;
}

export const PromptTable: FC<PromptTableProps> = ({
	timestamp,
	model,
	inputTokens,
	outputTokens,
	tokenUsageMetadata,
	className,
}) => {
	return (
		<div className={cn(className, "text-sm text-content-secondary")}>
			<div className="flex items-center justify-between mb-2">
				<span className="pr-4">Timestamp</span>
				<span
					className="font-mono whitespace-nowrap truncate"
					title={formatDate(timestamp)}
				>
					{formatDate(timestamp)}
				</span>
			</div>
			<div className="flex items-center justify-between mb-2">
				<span className="pr-4">Model</span>
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Badge className="gap-1.5">
								<AIBridgeModelIcon model={model} className="size-icon-xs" />
								<span className="truncate min-w-0">{model}</span>
							</Badge>
						</TooltipTrigger>
						<TooltipContent>{model}</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4">In / out tokens</span>
				<TokenBadges
					inputTokens={inputTokens}
					outputTokens={outputTokens}
					tokenUsageMetadata={tokenUsageMetadata}
				/>
			</div>
		</div>
	);
};
