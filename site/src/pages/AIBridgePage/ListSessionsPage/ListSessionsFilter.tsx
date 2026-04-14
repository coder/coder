import type { FC } from "react";
import {
	Filter,
	MenuSkeleton,
	type useFilter,
} from "#/components/Filter/Filter";
import { type UserFilterMenu, UserMenu } from "#/components/Filter/UserFilter";
import {
	ClientFilter,
	type ClientFilterMenu,
} from "../RequestLogsPage/RequestLogsFilter/ClientFilter";
import {
	ProviderFilter,
	type ProviderFilterMenu,
} from "../RequestLogsPage/RequestLogsFilter/ProviderFilter";

interface ListSessionsFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		// The user menu is hidden when data protection mode is active
		// to prevent leaking real usernames via the users API.
		user?: UserFilterMenu;
		provider: ProviderFilterMenu;
		client: ClientFilterMenu;
	};
}

export const ListSessionsFilter: FC<ListSessionsFilterProps> = ({
	filter,
	error,
	menus,
}) => {
	return (
		<Filter
			filter={filter}
			optionsSkeleton={<MenuSkeleton />}
			isLoading={menus.user?.isInitializing ?? false}
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
					{menus.user && <UserMenu menu={menus.user} placeholder="All users" />}
					<ProviderFilter menu={menus.provider} />
					<ClientFilter menu={menus.client} />
				</>
			}
		/>
	);
};
