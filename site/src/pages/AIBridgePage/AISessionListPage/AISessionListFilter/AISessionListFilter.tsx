import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import { OrganizationAutocomplete } from "components/OrganizationAutocomplete/OrganizationAutocomplete";
import type { FC } from "react";
import {
	ProviderFilter,
	type ProviderFilterMenu,
} from "../../RequestLogsPage/RequestLogsFilter/ProviderFilter";
import { ClientFilter, type ClientFilterMenu } from "./ClientFilter";

interface AISessionListFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		provider: ProviderFilterMenu;
		client: ClientFilterMenu;
	};
}

export const AISessionListFilter: FC<AISessionListFilterProps> = ({
	filter,
	error,
	menus,
}) => {
	return (
		<>
			<div className="mb-4 flex items-center justify-end">
				<span className="mr-2 text-content-secondary">Organization:</span>
				<OrganizationAutocomplete
					className="w-48"
					onChange={(org) => console.info("Selected organization", org)}
				/>
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
						<ClientFilter menu={menus.client} />
					</>
				}
			/>
		</>
	);
};
