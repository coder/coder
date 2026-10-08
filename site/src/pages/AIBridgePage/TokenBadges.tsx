import { ArrowDownIcon, ArrowUpIcon, type LucideIcon } from "lucide-react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { JsonPrettyPrinter } from "./JsonPrettyPrinter";
import { roundTokenDisplay } from "./utils";

type TokenCount = {
	icon: LucideIcon;
	title: string;
	label: string;
	value: number;
};

type TokenPairBadgesProps = {
	first: TokenCount;
	second: TokenCount;
	tokenUsageMetadata?: Record<string, unknown>;
};

const TokenPairBadges: React.FC<TokenPairBadgesProps> = ({
	first,
	second,
	tokenUsageMetadata,
}) => (
	<div className="flex items-center whitespace-nowrap">
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<span>
						<Badge className="gap-0.5 rounded-e-none">
							<first.icon className="size-icon-xs shrink-0" />
							<span className="truncate min-w-0">
								{roundTokenDisplay(first.value)}
							</span>
						</Badge>
						<Badge className="gap-0.5 bg-surface-tertiary rounded-s-none">
							<second.icon className="size-icon-xs shrink-0" />
							<span className="truncate min-w-0">
								{roundTokenDisplay(second.value)}
							</span>
						</Badge>
					</span>
				</TooltipTrigger>
				<TooltipContent>
					<div className="grid grid-cols-2 gap-8">
						{[first, second].map((count) => (
							<div key={count.title}>
								<div className="flex items-center gap-1">
									<count.icon className="size-icon-sm shrink-0" />
									<span className="text-content-primary text-sm">
										{count.title}
									</span>
								</div>
								<div className="flex items-center justify-between gap-4">
									<div className="text-sm text-content-secondary">
										{count.label}
									</div>
									<div className="text-sm text-content-secondary">
										{count.value.toLocaleString("en-US")}
									</div>
								</div>
							</div>
						))}
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

type TokenBadgesProps = {
	inputTokens: number;
	outputTokens: number;
	tokenUsageMetadata?: Record<string, unknown>;
};

export const TokenBadges: React.FC<TokenBadgesProps> = ({
	inputTokens,
	outputTokens,
	tokenUsageMetadata,
}) => (
	<TokenPairBadges
		first={{
			icon: ArrowDownIcon,
			title: "Input tokens",
			label: "Input",
			value: inputTokens,
		}}
		second={{
			icon: ArrowUpIcon,
			title: "Output tokens",
			label: "Output",
			value: outputTokens,
		}}
		tokenUsageMetadata={tokenUsageMetadata}
	/>
);

type CacheTokenBadgesProps = {
	cacheReadTokens: number;
	cacheWriteTokens: number;
};

export const CacheTokenBadges: React.FC<CacheTokenBadgesProps> = ({
	cacheReadTokens,
	cacheWriteTokens,
}) => (
	<TokenPairBadges
		first={{
			icon: ArrowDownIcon,
			title: "Cache read tokens",
			label: "Read",
			value: cacheReadTokens,
		}}
		second={{
			icon: ArrowUpIcon,
			title: "Cache write tokens",
			label: "Write",
			value: cacheWriteTokens,
		}}
	/>
);
