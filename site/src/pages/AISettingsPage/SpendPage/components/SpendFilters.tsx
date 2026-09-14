import type { FC } from "react";
import type { AIGatewaySpendFilter } from "#/api/typesGenerated";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";
import { spendFilterCategories } from "./spendFilterCategories";

// The chip keys match both the spend API query parameters and the sessions
// page filter keys, so a drill-in can hand its filters to the sessions link
// unchanged.
export type SpendDimensions = Pick<
	AIGatewaySpendFilter,
	"provider_name" | "client" | "model"
>;

interface SpendFiltersProps {
	// The combobox is query-string driven, like the workspaces filter: chips for
	// provider/client/model plus free text for the user search.
	filterQuery: string;
	onFilterQueryChange: (query: string) => void;
	categories?: readonly FilterCategory[];
	now?: Date;
	dateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	errorMessage?: string;
}

export const SpendFilters: FC<SpendFiltersProps> = ({
	filterQuery,
	onFilterQueryChange,
	categories = spendFilterCategories,
	now,
	dateRange,
	onDateRangeChange,
	errorMessage,
}) => {
	// The FilterCombobox renders its popover in-flow (disablePortal), so no
	// ancestor here may establish CSS containment: a `container-type` (e.g.
	// Tailwind's `@container`) would become the popover's containing block and
	// misposition it to the top-left. Plain flex-wrap keeps the combobox and date
	// picker on one line when there is room and stacks them when cramped.
	return (
		<div className="flex flex-wrap items-start gap-2">
			<FilterCombobox
				value={filterQuery}
				onChange={onFilterQueryChange}
				categories={categories}
				placeholder="Search and filter spend…"
				className="w-full min-w-60 flex-1 md:max-w-lg"
				errorMessage={errorMessage}
			/>
			<DateRangePicker
				now={now}
				value={dateRange}
				onChange={onDateRangeChange}
				size="lg"
			/>
		</div>
	);
};
