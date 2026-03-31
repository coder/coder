import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeModelIcon";
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
		<dl
			className={cn(
				"text-sm text-content-secondary m-0 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 items-center",
				"[&_dt]:whitespace-nowrap py-1",
				"[&_dt]:pr-4 [&_dt]:flex [&_dt]:items-center [&_dt]:h-6",
				"[&_dd]:m-0 [&_dd]:min-w-0 [&_dd]:h-6",
				className,
			)}
		>
			<dt>Timestamp</dt>
			<dd
				className="text-right flex items-center justify-end"
				title={formatDate(timestamp)}
			>
				<span className="block font-mono whitespace-nowrap truncate">
					{formatDate(timestamp)}
				</span>
			</dd>

			<dt>Model</dt>
			<dd className="flex justify-end">
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Badge className="gap-1.5 max-w-full min-w-0 overflow-hidden">
								<AIBridgeModelIcon model={model} className="size-icon-xs" />
								<span className="truncate min-w-0 flex-1">{model}</span>
							</Badge>
						</TooltipTrigger>
						<TooltipContent>{model}</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</dd>

			<dt>In / out tokens</dt>
			<dd className="flex justify-end">
				<TokenBadges
					inputTokens={inputTokens}
					outputTokens={outputTokens}
					tokenUsageMetadata={tokenUsageMetadata}
				/>
			</dd>
		</dl>
	);
};
