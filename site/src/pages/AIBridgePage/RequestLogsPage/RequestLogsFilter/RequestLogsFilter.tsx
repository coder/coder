import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import type { FC } from "react";
import { ModelFilter, type ModelFilterMenu } from "./ModelFilter";
import { ProviderFilter, type ProviderFilterMenu } from "./ProviderFilter";

interface RequestLogsFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		provider: ProviderFilterMenu;
		model: ModelFilterMenu;
	};
}

export const RequestLogsFilter: FC<RequestLogsFilterProps> = ({
	filter,
	error,
	menus,
}) => {
	return (
		<Filter
			filter={filter}
			optionsSkeleton={<MenuSkeleton />}
			isLoading={menus.user.isInitializing}
			presets={[
				{
					name: "All requests",
					query: "",
				},
				{
					name: "My requests",
					query: "initiator:me",
				},
			]}
			error={error}
			options={
				<>
					<UserMenu menu={menus.user} placeholder="All initiators" />
					<ProviderFilter menu={menus.provider} />
					<ModelFilter menu={menus.model} />
				</>
			}
		/>
	);
};
