/**
 * @file Defines a centralized place for filter dropdown groups that are
 * relevant across multiple pages within the Coder UI.
 *
 * @todo 2024-09-06 - Figure out how to move the user dropdown group into this
 * file (or whether there are enough subtle differences that it's not worth
 * centralizing the logic). We currently have two separate implementations for
 * the workspaces and audits page that have a risk of getting out of sync.
 */
import { API } from "api/api";
import { Avatar } from "components/Avatar/Avatar";
import {
	SelectFilter,
	type SelectFilterOption,
	SelectFilterSearch,
} from "components/Filter/SelectFilter";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import type { FC } from "react";

// Organization helpers ////////////////////////////////////////////////////////

export const useOrganizationsFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	return useFilterMenu({
		onChange,
		value,
		id: "organizations",
		getSelectedOption: async () => {
			if (value) {
				const organizations = await API.getOrganizations();
				const organization = organizations.find((o) => o.name === value);
				if (organization) {
					return {
						label: organization.display_name || organization.name,
						value: organization.name,
						startIcon: (
							<Avatar
								key={organization.id}
								size="sm"
								fallback={organization.display_name || organization.name}
								src={organization.icon}
							/>
						),
					};
				}
			}
			return null;
		},
		getOptions: async () => {
			// Only show the organizations for which you can view audit logs.
			const organizations = await API.getOrganizations();
			const permissions = await API.checkAuthorization({
				checks: Object.fromEntries(
					organizations.map((organization) => [
						organization.id,
						{
							object: {
								resource_type: "audit_log",
								organization_id: organization.id,
							},
							action: "read",
						},
					]),
				),
			});
			return organizations
				.filter((organization) => permissions[organization.id])
				.map<SelectFilterOption>((organization) => ({
					label: organization.display_name || organization.name,
					value: organization.name,
					startIcon: (
						<Avatar
							key={organization.id}
							size="sm"
							fallback={organization.display_name || organization.name}
							src={organization.icon}
						/>
					),
				}));
		},
	});
};

export type OrganizationsFilterMenu = ReturnType<
	typeof useOrganizationsFilterMenu
>;

interface OrganizationsMenuProps {
	menu: OrganizationsFilterMenu;
	width?: number;
}

export const OrganizationsMenu: FC<OrganizationsMenuProps> = ({
	menu,
	width,
}) => {
	return (
		<SelectFilter
			label="Select an organization"
			placeholder="All organizations"
			emptyText="No organizations found"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
			selectFilterSearch={
				<SelectFilterSearch
					inputProps={{ "aria-label": "Search organization" }}
					placeholder="Search organization..."
					value={menu.query}
					onChange={menu.setQuery}
				/>
			}
			width={width}
		/>
	);
};
