import { TriangleAlertIcon } from "lucide-react";
import type { OrganizationAISpendModel } from "#/api/typesGenerated";
import { Link as DocsLink } from "#/components/Link/Link";
import {
	Tooltip,
	TooltipContent,
	TooltipMessage,
	TooltipTitle,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { docs } from "#/utils/docs";

type UnpricedModelsTooltipProps = {
	displayName: string;
	models: readonly OrganizationAISpendModel[];
};

export const UnpricedModelsTooltip: React.FC<UnpricedModelsTooltipProps> = ({
	displayName,
	models,
}) => {
	const isSingle = models.length === 1;
	return (
		<Tooltip interactive>
			<TooltipTrigger
				aria-label={`${models.length} ${isSingle ? "model" : "models"} without pricing`}
				className="gap-1 text-sm tabular-nums text-content-warning"
			>
				<TriangleAlertIcon aria-hidden className="size-icon-sm" />
				{models.length}
			</TooltipTrigger>
			<TooltipContent className="max-w-[min(20rem,var(--radix-popper-available-width))]">
				<TooltipTitle>Model pricing missing</TooltipTitle>
				<TooltipMessage>
					{isSingle
						? `${models[0].name} used by ${displayName} has no pricing configured. Its usage isn't included, so total spend may be higher.`
						: `${models.length} models used by ${displayName} have no pricing configured. Their usage isn't included, so total spend may be higher.`}
				</TooltipMessage>
				<DocsLink
					size="sm"
					href={docs(
						"/ai-coder/ai-gateway/cost-controls#configure-model-prices",
					)}
					target="_blank"
					rel="noreferrer"
				>
					View docs
				</DocsLink>
			</TooltipContent>
		</Tooltip>
	);
};
