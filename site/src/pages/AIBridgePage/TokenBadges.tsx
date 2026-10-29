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
	Icon: LucideIcon;
	title: string;
	label: string;
	value: number;
};

type TokenCountPair = [TokenCount, TokenCount];

type TokenPairBadgeProps = {
	pair: TokenCountPair;
};

const TokenPairBadge: React.FC<TokenPairBadgeProps> = ({
	pair: [first, second],
}) => (
	<span className="flex items-center whitespace-nowrap">
		<Badge className="gap-0.5 rounded-e-none">
			<first.Icon className="size-icon-xs shrink-0" />
			<span className="truncate min-w-0">{roundTokenDisplay(first.value)}</span>
		</Badge>
		<Badge className="gap-0.5 bg-surface-tertiary rounded-s-none">
			<second.Icon className="size-icon-xs shrink-0" />
			<span className="truncate min-w-0">
				{roundTokenDisplay(second.value)}
			</span>
		</Badge>
	</span>
);

type TokenPairDetailsProps = {
	pair: TokenCountPair;
};

const TokenPairDetails: React.FC<TokenPairDetailsProps> = ({ pair }) => (
	<div className="grid grid-cols-2 gap-8">
		{pair.map((count) => (
			<div key={count.title}>
				<div className="flex items-center gap-1">
					<count.Icon className="size-icon-sm shrink-0" />
					<span className="text-content-primary text-sm">{count.title}</span>
				</div>
				<div className="flex items-center justify-between gap-4">
					<div className="text-sm text-content-secondary">{count.label}</div>
					<div className="text-sm text-content-secondary">
						{count.value.toLocaleString("en-US")}
					</div>
				</div>
			</div>
		))}
	</div>
);

type TokenBadgesProps = {
	inputTokens: number;
	outputTokens: number;
	cacheReadTokens?: number;
	cacheWriteTokens?: number;
	tokenUsageMetadata?: Record<string, unknown>;
};

export const TokenBadges: React.FC<TokenBadgesProps> = ({
	inputTokens,
	outputTokens,
	cacheReadTokens,
	cacheWriteTokens,
	tokenUsageMetadata,
}) => {
	const pairs: TokenCountPair[] = [
		[
			{
				Icon: ArrowDownIcon,
				title: "Input tokens",
				label: "Input",
				value: inputTokens,
			},
			{
				Icon: ArrowUpIcon,
				title: "Output tokens",
				label: "Output",
				value: outputTokens,
			},
		],
	];
	if (cacheReadTokens !== undefined && cacheWriteTokens !== undefined) {
		pairs.push([
			{
				Icon: ArrowDownIcon,
				title: "Cache read tokens",
				label: "Read",
				value: cacheReadTokens,
			},
			{
				Icon: ArrowUpIcon,
				title: "Cache write tokens",
				label: "Write",
				value: cacheWriteTokens,
			},
		]);
	}

	return (
		<div className="flex items-center whitespace-nowrap">
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<button
							type="button"
							className="m-0 flex flex-col items-end gap-1 whitespace-nowrap border-0 bg-transparent p-0 text-inherit rounded-md focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
						>
							{pairs.map((pair) => (
								<TokenPairBadge key={pair[0].title} pair={pair} />
							))}
						</button>
					</TooltipTrigger>
					<TooltipContent>
						<div className="flex flex-col gap-4">
							{pairs.map((pair) => (
								<TokenPairDetails key={pair[0].title} pair={pair} />
							))}
						</div>
						{tokenUsageMetadata && (
							<>
								<div className="text-content-primary text-sm mt-4">
									Token usage metadata
								</div>
								<pre className="mt-2 mb-1 p-4 bg-surface-secondary rounded overflow-x-auto">
									<JsonPrettyPrinter
										input={JSON.stringify(tokenUsageMetadata)}
									/>
								</pre>
							</>
						)}
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		</div>
	);
};
