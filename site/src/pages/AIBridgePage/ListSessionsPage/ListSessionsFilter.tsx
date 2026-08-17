import type { FC } from "react";
import {
	Filter,
	MenuSkeleton,
	type useFilter,
} from "#/components/Filter/Filter";
import { type UserFilterMenu, UserMenu } from "#/components/Filter/UserFilter";
import { ClientFilter, type ClientFilterMenu } from "../filters/ClientFilter";
import { ModelFilter, type ModelFilterMenu } from "../filters/ModelFilter";
import {
	ProviderFilter,
	type ProviderFilterMenu,
} from "../filters/ProviderFilter";
import { DateTimeRangeFilter } from "./DateTimeRangeFilter";
import type { TimeRange } from "./timeRange";

interface ListSessionsFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		provider: ProviderFilterMenu;
		client: ClientFilterMenu;
		model: ModelFilterMenu;
	};
	timeRange: TimeRange;
	defaultTimeRange: TimeRange;
	onTimeRangeChange: (range: TimeRange) => void;
}

export const ListSessionsFilter: FC<ListSessionsFilterProps> = ({
	filter,
	error,
	menus,
	timeRange,
	defaultTimeRange,
	onTimeRangeChange,
}) => {
	return (
		<Filter
			filter={filter}
			optionsSkeleton={<MenuSkeleton />}
			isLoading={menus.user.isInitializing}
			presets={[
				{
					name: "All sessions",
					query: "",
				},
				{
					name: "My sessions",
					query: "initiator:me",
				},
			]}
			error={error}
			options={
				<>
					<DateTimeRangeFilter
						value={timeRange}
						defaultValue={defaultTimeRange}
						onChange={onTimeRangeChange}
					/>
					<UserMenu menu={menus.user} placeholder="All users" />
					<ProviderFilter menu={menus.provider} />
					<ClientFilter menu={menus.client} />
					<ModelFilter menu={menus.model} />
				</>
			}
		/>
	);
};
