import type { FC } from "react";
import {
	Filter,
	MenuSkeleton,
	type useFilter,
} from "#/components/Filter/Filter";
import type { UseFilterMenuResult } from "#/components/Filter/menu";
import { SelectFilter } from "#/components/Filter/SelectFilter";
import { docs } from "#/utils/docs";

const userFilterQuery = {
	active: "status:active",
	serviceAccount: "service_account:true",
	all: "",
};

type StatusFilterMenu = UseFilterMenuResult;

const PRESET_FILTERS = [
	{ query: userFilterQuery.active, name: "Active users" },
	{ query: userFilterQuery.serviceAccount, name: "Service accounts" },
	{ query: userFilterQuery.all, name: "All users" },
];

type UsersFilterProps = {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus?: {
		status?: StatusFilterMenu;
	};
};

export const UsersFilter: FC<UsersFilterProps> = ({ filter, error, menus }) => {
	return (
		<Filter
			presets={PRESET_FILTERS}
			learnMoreLink={docs("/admin/users#user-filtering")}
			learnMoreLabel2="User status"
			learnMoreLink2={docs("/admin/users#user-status")}
			isLoading={menus?.status?.isInitializing ?? false}
			filter={filter}
			error={error}
			options={menus?.status && <StatusMenu {...menus.status} />}
			optionsSkeleton={menus?.status && <MenuSkeleton />}
		/>
	);
};

const StatusMenu = (menu: StatusFilterMenu) => {
	return (
		<SelectFilter
			label="Select a status"
			placeholder="All statuses"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
		/>
	);
};
