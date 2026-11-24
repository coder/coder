import { MaxAISpendPeriodDays } from "#/api/typesGenerated";
import { DateRangePicker } from "#/components/DateRangePicker/DateRangePicker";
import {
	fromPickerValue,
	type SpendDays,
	toPickerNow,
	toPickerValue,
} from "./spendDays";

type SpendDaysPickerProps = {
	now: Date | undefined;
	days: SpendDays;
	onDaysChange: (days: SpendDays) => void;
};

/** Picks whole UTC days, up to the longest period the spend endpoints allow. */
export const SpendDaysPicker: React.FC<SpendDaysPickerProps> = ({
	now,
	days,
	onDaysChange,
}) => (
	<DateRangePicker
		now={toPickerNow(now ?? new Date())}
		value={toPickerValue(days)}
		onChange={(value) => onDaysChange(fromPickerValue(value))}
		maxDays={MaxAISpendPeriodDays}
		size="lg"
	/>
);
