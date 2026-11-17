import {
	MaxAISpendPeriodDays,
	type OrganizationAISpendOverview,
	type UnpricedAIModel,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { DateRangePicker } from "#/components/DateRangePicker/DateRangePicker";
import {
	fromPickerValue,
	type SpendDays,
	toPickerNow,
	toPickerValue,
} from "./spendDays";
import { TotalSpendCard } from "./TotalSpendCard";

type OverviewTabProps = {
	now: Date | undefined;
	days: SpendDays;
	onDaysChange: (days: SpendDays) => void;
	overview: OrganizationAISpendOverview | undefined;
	overviewError: Error | null;
	unpricedModels: readonly UnpricedAIModel[] | undefined;
	isUnpricedModelsLoading: boolean;
};

export const OverviewTab: React.FC<OverviewTabProps> = ({
	now,
	days,
	onDaysChange,
	overview,
	overviewError,
	unpricedModels,
	isUnpricedModelsLoading,
}) => {
	return (
		<div className="flex flex-col gap-6">
			<div className="flex justify-end">
				<DateRangePicker
					now={toPickerNow(now ?? new Date())}
					value={toPickerValue(days)}
					onChange={(value) => onDaysChange(fromPickerValue(value))}
					maxDays={MaxAISpendPeriodDays}
					size="lg"
				/>
			</div>
			{overviewError !== null && <ErrorAlert error={overviewError} />}
			{(overview !== undefined || overviewError == null) && (
				<div className="grid gap-6 lg:grid-cols-2">
					<div className="flex flex-col gap-6">
						<TotalSpendCard
							spendMicros={overview?.totals.spend_micros}
							unpricedModels={unpricedModels}
							isUnpricedModelsLoading={isUnpricedModelsLoading}
						/>
					</div>
				</div>
			)}
		</div>
	);
};
