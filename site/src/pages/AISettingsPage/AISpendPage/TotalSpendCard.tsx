import { CoinsIcon, TriangleAlertIcon } from "lucide-react";
import type { UnpricedAIModel } from "#/api/typesGenerated";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { formatCostMicros } from "#/utils/currency";
import { StatCard } from "./StatCard";

type TotalSpendCardProps = {
	spendMicros: number | undefined;
	/** Undefined while loading, or when the first load fails (including a 403). */
	unpricedModels: readonly UnpricedAIModel[] | undefined;
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
				unpricedModels.length > 0 && (
					<p className="m-0 flex items-center gap-2 text-sm text-content-secondary">
						<TriangleAlertIcon
							aria-hidden
							className="size-icon-sm text-content-warning"
						/>
						{unpricedModels.length === 1
							? "1 Model not set up"
							: `${unpricedModels.length} Models not set up`}
					</p>
				)
			)}
		</StatCard>
	);
};
