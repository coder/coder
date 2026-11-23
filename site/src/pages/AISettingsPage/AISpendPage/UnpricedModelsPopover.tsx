import { cn } from "cn";
import { ChevronRightIcon, TriangleAlertIcon } from "lucide-react";
import { useId } from "react";
import type { UnpricedAIModel } from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { docs } from "#/utils/docs";

type UnpricedModelsPopoverProps = {
	models: readonly UnpricedAIModel[];
};

export const UnpricedModelsPopover: React.FC<UnpricedModelsPopoverProps> = ({
	models,
}) => {
	const headingId = useId();
	// TODO(AIGOV-730): adjust once the unpriced models endpoint is paginated.
	const isSingle = models.length === 1;
	const modelList = (
		<ul
			className={cn(
				"m-0 flex list-none flex-col gap-1",
				isSingle ? "p-0" : "p-2",
			)}
		>
			{models.map((model) => (
				<li
					key={`${model.provider_name}/${model.model}`}
					className={cn(
						"flex items-center gap-3 py-2 text-sm text-content-secondary",
						!isSingle && "px-2",
					)}
				>
					<ProviderIcon
						provider={model.provider_type}
						icon={model.provider_icon}
						alt={model.provider_display_name}
					/>
					{/* The same model can be unpriced on more than one provider. */}
					<span className="truncate">
						{model.provider_name}/{model.model}
					</span>
				</li>
			))}
		</ul>
	);
	return (
		<Popover>
			<PopoverTrigger asChild>
				<button
					type="button"
					className={cn(
						"flex w-fit items-center gap-2 rounded-sm border-none bg-transparent p-0",
						"text-sm text-content-secondary hover:text-content-primary",
						"focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
					)}
				>
					<TriangleAlertIcon
						aria-hidden
						className="size-icon-sm text-content-warning"
					/>
					{isSingle
						? "1 Model not set up"
						: `${models.length} Models not set up`}
					<ChevronRightIcon aria-hidden className="size-icon-sm" />
				</button>
			</PopoverTrigger>
			<PopoverContent
				align="start"
				aria-labelledby={headingId}
				className="flex w-96 flex-col gap-4 p-6"
			>
				<div className="flex flex-col gap-2">
					<h3 id={headingId} className="m-0 text-sm font-medium">
						Model pricing missing
					</h3>
					<p className="m-0 text-sm text-content-secondary">
						{isSingle
							? "This model has no pricing configured. Its usage isn't included, so total spend may be higher."
							: `${models.length} models used in this period have no pricing configured. Their usage isn't included, so total spend may be higher.`}
					</p>
				</div>
				{isSingle ? (
					modelList
				) : (
					<ScrollArea
						className="rounded-lg border border-border"
						viewportClassName="max-h-48 [&>div]:block!"
						viewportTabIndex={0}
						viewportAriaLabel="Models without pricing"
					>
						{modelList}
					</ScrollArea>
				)}
				<Link
					href={docs(
						"/ai-coder/ai-gateway/cost-controls#configure-model-prices",
					)}
					target="_blank"
					rel="noreferrer"
				>
					View docs
				</Link>
			</PopoverContent>
		</Popover>
	);
};
