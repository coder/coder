import { ArrowDownIcon, ArrowUpIcon } from "lucide-react";
import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { JsonPrettyPrinter } from "./JsonPrettyPrinter";
import { roundTokenDisplay } from "./utils";

interface TokenBadgesProps {
	size?: "xs" | "sm" | "md";
	inputTokens: number;
	outputTokens: number;
	tokenUsageMetadata?: Record<string, unknown>;
}

export const TokenBadges: FC<TokenBadgesProps> = ({
	size = "sm",
	inputTokens,
	outputTokens,
	tokenUsageMetadata,
}) => (
	<div className="flex items-center whitespace-nowrap">
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<span>
						<Badge className="gap-0 rounded-e-none" size={size}>
							<ArrowDownIcon className="size-icon-lg flex-shrink-0" />
							<span className="truncate min-w-0">
								{roundTokenDisplay(inputTokens)}
							</span>
						</Badge>
						<Badge
							className="gap-0 bg-surface-tertiary rounded-s-none"
							size={size}
						>
							<ArrowUpIcon className="size-icon-lg flex-shrink-0" />
							<span className="truncate min-w-0">
								{roundTokenDisplay(outputTokens)}
							</span>
						</Badge>
					</span>
				</TooltipTrigger>
				<TooltipContent>
					<div className="grid grid-cols-2 gap-8">
						<div>
							<div className="flex items-center gap-1">
								<ArrowDownIcon className="size-icon-sm flex-shrink-0" />
								<span className="text-content-primary text-sm">
									Input tokens
								</span>
							</div>
							<div className="flex items-center justify-between gap-4">
								<div className="text-sm text-content-secondary">Input</div>
								<div className="text-sm text-content-secondary">
									{inputTokens.toLocaleString()}
								</div>
							</div>
						</div>

						<div>
							<div className="flex items-center gap-1">
								<ArrowUpIcon className="size-icon-sm flex-shrink-0" />
								<span className="text-content-primary text-sm">
									Output tokens
								</span>
							</div>
							<div className="flex items-center justify-between gap-4">
								<div className="text-sm text-content-secondary">Output</div>
								<div className="text-sm text-content-secondary">
									{outputTokens.toLocaleString()}
								</div>
							</div>
						</div>
					</div>
					{tokenUsageMetadata && (
						<>
							<div className="text-content-primary text-sm mt-4">
								Token usage metadata
							</div>
							<pre className="mt-2 mb-1 p-4 bg-surface-secondary rounded overflow-x-auto">
								<JsonPrettyPrinter input={JSON.stringify(tokenUsageMetadata)} />
							</pre>
						</>
					)}
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	</div>
);
