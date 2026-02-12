import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import dayjs from "dayjs";
import { ChevronDownIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";
import type { DateRangeValue } from "./DateRange";
import { lastWeeks } from "./utils";

// There is no point in showing the period > 6 months. We prune stats older than
// 6 months.
export const numberOfWeeksOptions = [4, 12, 24] as const;

interface WeekPickerProps {
	value: DateRangeValue;
	onChange: (value: DateRangeValue) => void;
}

export const WeekPicker: FC<WeekPickerProps> = ({ value, onChange }) => {
	const numberOfWeeks = dayjs(value.endDate).diff(
		dayjs(value.startDate),
		"week",
	);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" className="group">
					Last {numberOfWeeks} weeks
					<ChevronDownIcon
						className={cn(
							"!size-icon-xs",
							"group-data-[state=open]:rotate-180 transition-transform",
						)}
					/>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start">
				<DropdownMenuRadioGroup
					value={String(numberOfWeeks)}
					onValueChange={(v) => onChange(lastWeeks(Number(v)))}
				>
					{numberOfWeeksOptions.map((option) => (
						<DropdownMenuRadioItem key={option} value={String(option)}>
							Last {option} weeks
						</DropdownMenuRadioItem>
					))}
				</DropdownMenuRadioGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
