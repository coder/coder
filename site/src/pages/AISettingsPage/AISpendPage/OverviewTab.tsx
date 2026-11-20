import { CoinsIcon, UsersIcon } from "lucide-react";
import type {
	OrganizationAISpendOverview,
	OrganizationAISpendUser,
	UnpricedAIModel,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { formatCostMicros } from "#/utils/currency";
import { SpendDaysPicker } from "./SpendDaysPicker";
import { SpendTrendsCard } from "./SpendTrendsCard";
import { StatCard } from "./StatCard";
import type { SpendDays } from "./spendDays";
import { TopSpendersCard } from "./TopSpendersCard";
import { TotalSpendCard } from "./TotalSpendCard";

type OverviewTabProps = {
	now: Date | undefined;
	days: SpendDays;
	onDaysChange: (days: SpendDays) => void;
	overview: OrganizationAISpendOverview | undefined;
	overviewError: Error | null;
	unpricedModels: readonly UnpricedAIModel[] | undefined;
	isUnpricedModelsLoading: boolean;
	topSpenderDetails: readonly OrganizationAISpendUser[] | undefined;
	topSpenderDetailsError: Error | null;
	organizationName: string;
	canViewSessions: boolean;
	onOpenDetailedReport: (username?: string) => void;
};

export const OverviewTab: React.FC<OverviewTabProps> = ({
	now,
	days,
	onDaysChange,
	overview,
	overviewError,
	unpricedModels,
	isUnpricedModelsLoading,
	topSpenderDetails,
	topSpenderDetailsError,
	organizationName,
	canViewSessions,
	onOpenDetailedReport,
}) => {
	return (
		<div className="flex flex-col gap-6">
			<div className="flex justify-end">
				<SpendDaysPicker now={now} days={days} onDaysChange={onDaysChange} />
			</div>
			{overviewError !== null && <ErrorAlert error={overviewError} />}
			{(overview !== undefined || overviewError === null) && (
				<div className="grid gap-6 lg:grid-cols-2">
					<div className="flex flex-col gap-6">
						<TotalSpendCard
							spendMicros={overview?.totals.spend_micros}
							unpricedModels={unpricedModels}
							isUnpricedModelsLoading={isUnpricedModelsLoading}
						/>
						<div className="grid gap-6 sm:grid-cols-2">
							<StatCard
								icon={UsersIcon}
								label="Active users"
								value={overview?.count.toLocaleString("en-US")}
							/>
							<StatCard
								icon={CoinsIcon}
								label="Median spend"
								value={
									overview === undefined
										? undefined
										: formatCostMicros(overview.totals.median_spend_micros)
								}
							/>
						</div>
						<SpendTrendsCard days={days} intervals={overview?.intervals} />
					</div>
					<TopSpendersCard
						overview={overview}
						topSpenderDetails={topSpenderDetails}
						topSpenderDetailsError={topSpenderDetailsError}
						organizationName={organizationName}
						canViewSessions={canViewSessions}
						onOpenDetailedReport={onOpenDetailedReport}
					/>
				</div>
			)}
		</div>
	);
};
