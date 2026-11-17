import { MaxAISpendPeriodDays } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { DateRangePicker } from "#/components/DateRangePicker/DateRangePicker";
import {
	fromPickerValue,
	type SpendDays,
	toPickerNow,
	toPickerValue,
} from "./spendDays";

type OverviewTabProps = {
	now: Date | undefined;
	days: SpendDays;
	onDaysChange: (days: SpendDays) => void;
	overviewError: unknown;
};

export const OverviewTab: React.FC<OverviewTabProps> = ({
	now,
	days,
	onDaysChange,
	overviewError,
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
			{overviewError != null && <ErrorAlert error={overviewError} />}
		</div>
	);
};
