import { Badge } from "components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ArrowDownIcon, ArrowUpIcon } from "lucide-react";
import type { FC } from "react";
import { formatDate } from "utils/time";
import { AIBridgeModelIcon } from "../RequestLogsPage/icons/AIBridgeModelIcon";
import { roundDurationDisplay } from "../utils";

interface TokenBadgesProps {
	inputTokens: number;
	outputTokens: number;
}

export const TokenBadges: FC<TokenBadgesProps> = ({
	inputTokens,
	outputTokens,
}) => (
	<div className="flex items-center">
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<Badge className="gap-0 rounded-e-none">
						<ArrowDownIcon className="size-icon-lg flex-shrink-0" />
						<span className="truncate min-w-0">{inputTokens}</span>
					</Badge>
				</TooltipTrigger>
				<TooltipContent>{inputTokens} input tokens</TooltipContent>
			</Tooltip>
		</TooltipProvider>
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<Badge className="gap-0 bg-surface-tertiary rounded-s-none">
						<ArrowUpIcon className="size-icon-lg flex-shrink-0" />
						<span className="truncate min-w-0">{outputTokens}</span>
					</Badge>
				</TooltipTrigger>
				<TooltipContent>{outputTokens} output tokens</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	</div>
);

interface PromptDetailsTableProps {
	timestamp: Date;
	model: string;
	inputTokens: number;
	outputTokens: number;
}

export const PromptDetailsTable: FC<PromptDetailsTableProps> = ({
	timestamp,
	model,
	inputTokens,
	outputTokens,
}) => {
	return (
		<div className="w-64 text-sm text-content-secondary">
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
				<TokenBadges inputTokens={inputTokens} outputTokens={outputTokens} />
			</div>
		</div>
	);
};

interface AgenticLoopDetailsTableProps {
	duration: number; // in seconds
	toolCalls: number;
	inputTokens: number;
	outputTokens: number;
}

export const AgenticLoopDetailsTable: FC<AgenticLoopDetailsTableProps> = ({
	duration,
	toolCalls,
	inputTokens,
	outputTokens,
}) => {
	return (
		<div className="w-64 text-sm text-content-secondary">
			<div className="flex items-center justify-between">
				<span className="pr-4">In / out tokens</span>
				<TokenBadges inputTokens={inputTokens} outputTokens={outputTokens} />
			</div>
			<div className="flex items-center justify-between my-2">
				<span className="pr-4">Tool calls</span>
				<span>{toolCalls}</span>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4">Duration</span>
				<span title={`${duration}ms`}>
					{roundDurationDisplay(duration)} seconds
				</span>
			</div>
		</div>
	);
};
