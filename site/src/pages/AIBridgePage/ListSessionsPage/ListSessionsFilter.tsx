import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import {
	ProviderFilter,
	type ProviderFilterMenu,
} from "pages/AIBridgePage/RequestLogsPage/RequestLogsFilter/ProviderFilter";
import type { FC } from "react";

interface ListSessionsFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		provider: ProviderFilterMenu;
	};
}

export const ListSessionsFilter: FC<ListSessionsFilterProps> = ({
	filter,
	error,
	menus,
}) => {
	return (
		<>
			<div className="mb-4 flex items-center justify-end">
				<span className="mr-2 text-content-secondary">Organization:</span>
				{/* TODO: add organization filter */}
				{/* <OrganizationAutocomplete */}
				{/* 	onChange={(org) => console.info("Selected organization", org)} */}
				{/* /> */}
			</div>
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
						<UserMenu menu={menus.user} placeholder="All users" />
						<ProviderFilter menu={menus.provider} />
						{/* TODO: add client filter */}
						{/* <ClientFilter menu={menus.client} /> */}
					</>
				}
			/>
		</>
	);
};
