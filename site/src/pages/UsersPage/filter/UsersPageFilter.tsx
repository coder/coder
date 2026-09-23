import { CircleDotIcon, ShieldIcon, UsersIcon } from "lucide-react";
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
import {
	getRoleFilterOptions,
	getStatusFilterOptions,
	getUserTypeFilterOptions,
	USER_TYPE_CHIP_KEYS,
} from "./categoryOptions";
import {
	LAST_SEEN_AFTER_KEY,
	LAST_SEEN_BEFORE_KEY,
	LAST_SEEN_KEYS,
	lastSeenPresets,
} from "./lastSeenRange";

type UsersPageFilterProps = Readonly<{
	filter: UseFilterResult;
	error?: unknown;
	lastSeen: DateTimeRangeValue;
	onLastSeenChange: (value: DateTimeRangeValue) => void;
}>;

export const UsersPageFilter: FC<UsersPageFilterProps> = ({
	filter,
	error,
	lastSeen,
	onLastSeenChange,
}) => {
	const queryClient = useQueryClient();

	const categories = useMemo<FilterCategory[]>(
		() => [
			{
				key: "status",
				label: "Status",
				icon: <CircleDotIcon />,
				inlineOptions: true,
				inlineOptionIcons: true,
				getOptions: getStatusFilterOptions,
			},
			{
				key: "role",
				label: "Role",
				icon: <ShieldIcon />,
				getOptions: (query) => getRoleFilterOptions(query, queryClient),
			},
			{
				// FilterCombobox shows `attribute` chips as the option label only.
				key: "attribute",
				label: "User type",
				aliases: ["type", "user_type"],
				icon: <UsersIcon />,
				chipKeys: USER_TYPE_CHIP_KEYS,
				inlineOptions: true,
				inlineOptionsLabel: "User type is…",
				getOptions: getUserTypeFilterOptions,
			},
		],
		[queryClient],
	);

	// Keep the last seen keys, which the combobox value omits.
	const handleChange = useCallback(
		(query: string) => {
			const values = parseFilterQuery(filter.query);
			const lastSeenQuery = stringifyFilter({
				[LAST_SEEN_AFTER_KEY]: values[LAST_SEEN_AFTER_KEY],
				[LAST_SEEN_BEFORE_KEY]: values[LAST_SEEN_BEFORE_KEY],
			});
			filter.update(
				[query, lastSeenQuery].filter((part) => part.length > 0).join(" "),
			);
		},
		[filter],
	);

	const showValidationError = hasError(error) && isApiValidationError(error);

	return (
		<div className="mb-4 flex flex-wrap items-start gap-2">
			<div className="flex w-full min-w-0 max-w-full flex-col gap-2 sm:w-auto">
				<FilterCombobox
					value={extractFreeText(filter.query, LAST_SEEN_KEYS)}
					onChange={handleChange}
					categories={categories}
					placeholder="Search and filter users…"
					className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
					errorMessage={
						showValidationError ? getValidationErrorMessage(error) : undefined
					}
				/>
			</div>
			<DateTimeRangePicker
				value={lastSeen}
				onChange={onLastSeenChange}
				presets={lastSeenPresets}
				label="Last seen"
				size="lg"
			/>
		</div>
	);
};
