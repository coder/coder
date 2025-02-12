import { AuditActions, ResourceTypes } from "api/typesGenerated";
import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import {
	SelectFilter,
	type SelectFilterOption,
} from "components/Filter/SelectFilter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import capitalize from "lodash/capitalize";
import {
	type OrganizationsFilterMenu,
	OrganizationsMenu,
} from "modules/tableFiltering/options";
import type { FC } from "react";
import { docs } from "utils/docs";

const PRESET_FILTERS = [
	{
		query: "resource_type:workspace action:create",
		name: "Created workspaces",
	},
	{ query: "resource_type:template action:create", name: "Added templates" },
	{ query: "resource_type:user action:delete", name: "Deleted users" },
	{
		query: "resource_type:workspace_build action:start build_reason:initiator",
		name: "Builds started by a user",
	},
	{
		query: "resource_type:api_key action:login",
		name: "User logins",
	},
];

interface AuditFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		action: ActionFilterMenu;
		resourceType: ResourceTypeFilterMenu;
		// The organization menu is only provided in a multi-org setup.
		organization?: OrganizationsFilterMenu;
	};
}

export const AuditFilter: FC<AuditFilterProps> = ({ filter, error, menus }) => {
	const width = menus.organization ? 175 : undefined;

	return (
		<Filter
			learnMoreLink={docs("/admin/security/audit-logs#filtering-logs")}
			presets={PRESET_FILTERS}
			isLoading={menus.user.isInitializing}
			filter={filter}
			error={error}
			options={
				<>
					<ResourceTypeMenu width={width} menu={menus.resourceType} />
					<ActionMenu width={width} menu={menus.action} />
					<UserMenu width={width} menu={menus.user} />
					{menus.organization && (
						<OrganizationsMenu width={width} menu={menus.organization} />
					)}
				</>
			}
			optionsSkeleton={
				<>
					<MenuSkeleton />
					<MenuSkeleton />
					<MenuSkeleton />
					{menus.organization && <MenuSkeleton />}
				</>
			}
		/>
	);
};

export const useActionFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	const actionOptions: SelectFilterOption[] = AuditActions.map((action) => ({
		value: action,
		label: capitalize(action),
	}));
	return useFilterMenu({
		onChange,
		value,
		id: "status",
		getSelectedOption: async () =>
			actionOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => actionOptions,
	});
};

export type ActionFilterMenu = ReturnType<typeof useActionFilterMenu>;

interface ActionMenuProps {
	menu: ActionFilterMenu;
	width?: number;
}

const ActionMenu: FC<ActionMenuProps> = ({ menu, width }) => {
	return (
		<SelectFilter
			label="Select an action"
			placeholder="All actions"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
			width={width}
		/>
	);
};

export const useResourceTypeFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	const actionOptions: SelectFilterOption[] = ResourceTypes.map((type) => {
		let label: string = capitalize(type);

		if (type === "api_key") {
			label = "API Key";
		}

		if (type === "git_ssh_key") {
			label = "Git SSH Key";
		}

		if (type === "template_version") {
			label = "Template Version";
		}

		if (type === "workspace_build") {
			label = "Workspace Build";
		}

		return {
			value: type,
			label,
		};
	});
	return useFilterMenu({
		onChange,
		value,
		id: "resourceType",
		getSelectedOption: async () =>
			actionOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => actionOptions,
	});
};

export type ResourceTypeFilterMenu = ReturnType<
	typeof useResourceTypeFilterMenu
>;

interface ResourceTypeMenuProps {
	menu: ResourceTypeFilterMenu;
	width?: number;
}

const ResourceTypeMenu: FC<ResourceTypeMenuProps> = ({ menu, width }) => {
	return (
		<SelectFilter
			label="Select a resource type"
			placeholder="All resource types"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
			width={width}
		/>
	);
};
