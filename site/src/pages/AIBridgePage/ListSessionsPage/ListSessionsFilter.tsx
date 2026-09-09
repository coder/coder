import { BotIcon, CpuIcon, MonitorIcon, UserIcon } from "lucide-react";
import { type FC, useCallback, useMemo } from "react";
import { useQueryClient } from "react-query";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import { DateTimeRangePicker } from "#/components/DateTimeRangePicker/DateTimeRangePicker";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import { extractFreeText } from "#/components/Filter/FilterCombobox/filterQuery";
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";
import {
	parseFilterQuery,
	stringifyFilter,
} from "#/components/Filter/filterQuery";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	getClientFilterOptions,
	getInitiatorFilterOptions,
	getModelFilterOptions,
	getProviderFilterOptions,
} from "../filters/categoryOptions";

// The time range shares the `filter` query string with the combobox filters,
// but the picker owns these two keys: they are split out of the combobox value
// and merged back on every change so they never surface as chips or free text.
const TIME_RANGE_KEYS = ["started_after", "started_before"] as const;

type ListSessionsFilterProps = Readonly<{
	filter: UseFilterResult;
	error?: unknown;
	timeRange: DateTimeRangeValue;
	onTimeRangeChange: (value: DateTimeRangeValue) => void;
}>;

export const ListSessionsFilter: FC<ListSessionsFilterProps> = ({
	filter,
	error,
	timeRange,
	onTimeRangeChange,
}) => {
	const { user: me } = useAuthenticated();
	const queryClient = useQueryClient();

	const categories = useMemo<FilterCategory[]>(
		() => [
			{
				key: "initiator",
				label: "User",
				aliases: ["user"],
				icon: <UserIcon />,
				getOptions: (query) =>
					getInitiatorFilterOptions(query, me, queryClient),
			},
			{
				key: "provider_name",
				label: "Provider",
				aliases: ["provider"],
				icon: <BotIcon />,
				getOptions: getProviderFilterOptions,
			},
			{
				key: "client",
				label: "Client",
				icon: <MonitorIcon />,
				getOptions: getClientFilterOptions,
			},
			{
				key: "model",
				label: "Model",
				icon: <CpuIcon />,
				getOptions: getModelFilterOptions,
			},
		],
		[me, queryClient],
	);

	const handleChange = useCallback(
		(query: string) => {
			const values = parseFilterQuery(filter.query);
			const timeRangeQuery = stringifyFilter({
				started_after: values.started_after,
				started_before: values.started_before,
			});
			filter.update(
				[query, timeRangeQuery].filter((part) => part.length > 0).join(" "),
			);
		},
		[filter],
	);

	// The page renders no error alert of its own, so the filter surfaces the
	// actionable "invalid query" message.
	const showValidationError = hasError(error) && isApiValidationError(error);

	return (
		<div className="flex flex-wrap items-start gap-2">
			{/* Column wrapper so the validation message sits under the input
			    instead of beside it in the row. */}
			<div className="flex min-w-0 max-w-full flex-col gap-2">
				<FilterCombobox
					value={extractFreeText(filter.query, TIME_RANGE_KEYS)}
					onChange={handleChange}
					categories={categories}
					placeholder="Search and filter sessions…"
					// Starts at a compact width and widens to fit chips before wrapping.
					className="w-auto min-w-lg max-w-full self-start"
					errorMessage={
						showValidationError ? getValidationErrorMessage(error) : undefined
					}
				/>
			</div>
			<DateTimeRangePicker
				value={timeRange}
				onChange={onTimeRangeChange}
				size="lg"
			/>
		</div>
	);
};
