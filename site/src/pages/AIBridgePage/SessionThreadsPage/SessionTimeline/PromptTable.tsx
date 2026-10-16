import { cn } from "cn";
import type { AIBridgePricedModel } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import {
	Tooltip,
	TooltipContent,
	TooltipMessage,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatPricePerMillionTokens } from "#/modules/aiModels/knownModels/catalog";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";
import { microsToDollars } from "#/utils/currency";
import { formatDate } from "#/utils/time";
import { CostBadge } from "../../CostBadge";
import { CacheTokenBadges, TokenBadges } from "../../TokenBadges";

const formatPrice = (micros: number | null): string => {
	if (micros === null) {
		return "Not set";
	}
	const price = formatPricePerMillionTokens(microsToDollars(micros));
	return `${price.belowThreshold ? "< " : ""}$${price.value} / 1M tokens`;
};

type ModelBadgeProps = {
	model: string;
	pricedModel?: AIBridgePricedModel;
};

const ModelBadge: React.FC<ModelBadgeProps> = ({ model, pricedModel }) => {
	const prices: [string, number | null][] = pricedModel
		? [
				["Input", pricedModel.input_price],
				["Output", pricedModel.output_price],
				["Cache read", pricedModel.cache_read_price],
				["Cache write", pricedModel.cache_write_price],
			]
		: [];

	let warning: string | undefined;
	if (!pricedModel) {
		warning = "No price was recorded for this model.";
	} else if (pricedModel.model !== model) {
		warning = `Priced as ${pricedModel.model}, the model reported by the provider.`;
	}

	return (
		<span className="inline-flex items-center gap-1 min-w-0 max-w-full">
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<Badge
							asChild
							className="gap-1.5 max-w-full min-w-0 overflow-hidden"
						>
							<button type="button" className="cursor-default">
								<AIBridgeModelIcon model={model} className="size-icon-xs" />
								<span className="truncate min-w-0 flex-1">{model}</span>
							</button>
						</Badge>
					</TooltipTrigger>
					<TooltipContent>
						<div className="text-content-primary text-sm">
							{pricedModel?.model ?? model}
						</div>
						{pricedModel ? (
							<dl className="m-0 mt-2 grid grid-cols-[auto_auto] gap-x-4 gap-y-1 text-sm text-content-secondary">
								{prices.map(([label, micros]) => (
									<div key={label} className="contents">
										<dt>{label}</dt>
										<dd className="m-0 text-right">{formatPrice(micros)}</dd>
									</div>
								))}
							</dl>
						) : (
							<div className="mt-1 text-sm text-content-secondary">
								No price recorded
							</div>
						)}
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>
			{warning && (
				<InfoTooltip type="warning" size="small" ariaLabel="Pricing warning">
					<TooltipMessage>{warning}</TooltipMessage>
				</InfoTooltip>
			)}
		</span>
	);
};

type PromptTableProps = {
	timestamp: Date;
	model: string;
	pricedModel?: AIBridgePricedModel;
	inputTokens: number;
	outputTokens: number;
	cacheReadTokens: number;
	cacheWriteTokens: number;
	costMicros: number;
	hasUnpricedUsage: boolean;
	tokenUsageMetadata?: Record<string, unknown>;
	className?: string;
};

export const PromptTable: React.FC<PromptTableProps> = ({
	timestamp,
	model,
	pricedModel,
	inputTokens,
	outputTokens,
	cacheReadTokens,
	cacheWriteTokens,
	costMicros,
	hasUnpricedUsage,
	tokenUsageMetadata,
	className,
}) => {
	return (
		<dl
			className={cn(
				"text-sm text-content-secondary font-normal m-0 flex flex-col gap-y-2 py-1",
				className,
			)}
		>
			<div className="flex items-center justify-between">
				<dt className="shrink-0 whitespace-nowrap">Timestamp</dt>
				<dd
					className="ml-4 min-w-0 truncate font-mono text-xs"
					title={formatDate(timestamp)}
				>
					{formatDate(timestamp)}
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 whitespace-nowrap">Model</dt>
				<dd className="ml-4 min-w-0 truncate flex justify-end">
					<ModelBadge model={model} pricedModel={pricedModel} />
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 whitespace-nowrap">In / out tokens</dt>
				<dd className="ml-4 min-w-0 truncate flex justify-end">
					<TokenBadges
						inputTokens={inputTokens}
						outputTokens={outputTokens}
						tokenUsageMetadata={tokenUsageMetadata}
					/>
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 whitespace-nowrap">Cache read / write</dt>
				<dd className="ml-4 min-w-0 truncate flex justify-end">
					<CacheTokenBadges
						cacheReadTokens={cacheReadTokens}
						cacheWriteTokens={cacheWriteTokens}
					/>
				</dd>
			</div>

			<div className="flex items-center justify-between">
				<dt className="shrink-0 whitespace-nowrap">Cost</dt>
				<dd className="ml-4 min-w-0 truncate flex justify-end">
					<CostBadge
						costMicros={costMicros}
						hasUnpricedUsage={hasUnpricedUsage}
					/>
				</dd>
			</div>
		</dl>
	);
};
