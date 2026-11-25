import { CoinsIcon } from "lucide-react";
import type { UnpricedAIModels } from "#/api/typesGenerated";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { formatCostMicros } from "#/utils/currency";
import { StatCard } from "./StatCard";
import { UnpricedModelsPopover } from "./UnpricedModelsPopover";

type TotalSpendCardProps = {
	spendMicros: number | undefined;
	/** Undefined while loading, or when the first load fails (including a 403). */
	unpricedModels: UnpricedAIModels | undefined;
	isUnpricedModelsLoading: boolean;
};

export const TotalSpendCard: React.FC<TotalSpendCardProps> = ({
	spendMicros,
	unpricedModels,
	isUnpricedModelsLoading,
}) => {
	return (
		<StatCard
			icon={CoinsIcon}
			label="Total spend"
			value={
				spendMicros === undefined ? undefined : formatCostMicros(spendMicros)
			}
		>
			{isUnpricedModelsLoading ? (
				<div className="flex h-6 items-center gap-2">
					<Spinner loading size="sm" label="Checking model pricing" />
					<Skeleton variant="circular" className="h-3 w-48" />
					<Skeleton variant="circular" className="size-3" />
				</div>
			) : (
				unpricedModels !== undefined &&
				unpricedModels.count > 0 && (
					<UnpricedModelsPopover
						models={unpricedModels.models}
						count={unpricedModels.count}
					/>
				)
			)}
		</StatCard>
	);
};
