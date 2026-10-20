import { BrainIcon, CloudIcon, TerminalIcon, UserIcon } from "lucide-react";
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
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { getUserFilterOptions } from "#/pages/WorkspacesPage/filter/categoryOptions";
import {
	getClientFilterOptions,
	getModelFilterOptions,
	getProviderFilterOptions,
} from "./categoryOptions";
import { queryWithSearch, withoutTimeRange } from "./timeRange";

type ListSessionsFilterProps = {
	filter: UseFilterResult;
	error?: unknown;
	timeRange: DateTimeRangeValue;
	onTimeRangeChange: (value: DateTimeRangeValue) => void;
};

export const ListSessionsFilter: React.FC<ListSessionsFilterProps> = ({
	filter,
	error,
	timeRange,
	onTimeRangeChange,
}) => {
	const { user: me } = useAuthenticated();
	const queryClient = useQueryClient();

	const categories: FilterCategory[] = [
		{
			key: "initiator",
			label: "User",
			icon: <UserIcon />,
			getOptions: (query) => getUserFilterOptions(query, me, queryClient),
		},
		{
			key: "provider_name",
			label: "Provider",
			icon: <CloudIcon />,
			getOptions: (query) => getProviderFilterOptions(query, queryClient),
		},
		{
			key: "client",
			label: "Client",
			icon: <TerminalIcon />,
			getOptions: (query) => getClientFilterOptions(query, queryClient),
		},
		{
			key: "model",
			label: "Model",
			icon: <BrainIcon />,
			getOptions: (query) => getModelFilterOptions(query, queryClient),
		},
	];

	// The page hides its ErrorAlert for API validation errors, so the filter
	// owns surfacing the actionable "invalid query" message.
	const showValidationError = hasError(error) && isApiValidationError(error);

	return (
		<div className="flex flex-wrap items-start gap-2 lg:flex-nowrap">
			<div className="flex min-w-0 flex-1 basis-full flex-col gap-2 lg:basis-auto">
				<FilterCombobox
					value={withoutTimeRange(filter.query)}
					onChange={(search) =>
						filter.update(queryWithSearch(filter.query, search))
					}
					categories={categories}
					placeholder="Search and filter sessions…"
					className="w-full min-w-0"
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
