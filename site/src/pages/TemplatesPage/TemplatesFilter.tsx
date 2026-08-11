import type { FC } from "react";
import { API } from "#/api/api";
import type { Organization } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	Filter,
	MenuSkeleton,
	type UseFilterResult,
	useFilter,
} from "#/components/Filter/Filter";
import { useFilterMenu } from "#/components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "#/components/Filter/SelectFilter";
import {
	DEFAULT_USER_FILTER_WIDTH,
	type UserFilterMenu,
	UserMenu,
	useUserFilterMenu,
} from "#/components/Filter/UserFilter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";

export type TemplateFilterState = {
	filter: UseFilterResult;
	menus: {
		user?: ReturnType<typeof useUserFilterMenu>;
	};
};

type UseTemplatesFilterOptions = {
	searchParams: URLSearchParams;
	onSearchParamsChange: (params: URLSearchParams) => void;
	enabled?: boolean;
};

export const useTemplatesFilter = ({
	searchParams,
	onSearchParamsChange,
	enabled = true,
}: UseTemplatesFilterOptions): TemplateFilterState => {
	const filter = useFilter({
		searchParams,
		onSearchParamsChange,
	});

	const { permissions } = useAuthenticated();
	const canFilterByUser = permissions.viewAllUsers;
	const userMenu = useUserFilterMenu({
		value: filter.values.author,
		onChange: (option) =>
			filter.update({ ...filter.values, author: option?.value }),
		enabled: enabled && canFilterByUser,
	});

	return {
		filter,
		menus: {
			user: canFilterByUser ? userMenu : undefined,
		},
	};
};

interface TemplatesFilterProps {
	filter: UseFilterResult;
	error?: unknown;

	userMenu?: UserFilterMenu;
}

export const TemplatesFilter: FC<TemplatesFilterProps> = ({
	filter,
	error,
	userMenu,
}) => {
	const { showOrganizations } = useDashboard();
	const width = showOrganizations ? DEFAULT_USER_FILTER_WIDTH : undefined;
	const organizationMenu = useFilterMenu({
		onChange: (option) =>
			filter.update({ ...filter.values, organization: option?.value }),
		value: filter.values.organization,
		id: "organization",
		getSelectedOption: async () => {
			if (!filter.values.organization) {
				return null;
			}

			const org = await API.getOrganization(filter.values.organization);
			return orgOption(org);
		},
		getOptions: async () => {
			const orgs = await API.getMyOrganizations();
			return orgs.map(orgOption);
		},
	});

	return (
		<Filter
			presets={[
				{ query: "", name: "All templates" },
				{ query: "author:me", name: "Templates you authored" },
				{ query: "deprecated:true", name: "Deprecated templates" },
			]}
			// TODO: Add docs for this
			// learnMoreLink={docs("/admin/templates#template-filtering")}
			isLoading={false}
			filter={filter}
			error={error}
			options={
				<>
					{userMenu && <UserMenu width={width} menu={userMenu} />}
					<SelectFilter
						placeholder="All organizations"
						label="Select an organization"
						options={organizationMenu.searchOptions}
						selectedOption={organizationMenu.selectedOption ?? undefined}
						onSelect={organizationMenu.selectOption}
					/>
				</>
			}
			optionsSkeleton={
				<>
					{userMenu && <MenuSkeleton />}
					<MenuSkeleton />
				</>
			}
		/>
	);
};

const orgOption = (org: Organization): SelectFilterOption => ({
	label: org.display_name || org.name,
	value: org.name,
	startIcon: (
		<Avatar key={org.id} size="sm" fallback={org.display_name} src={org.icon} />
	),
});
