import { API } from "api/api";
import type { Template, WorkspaceStatus } from "api/typesGenerated";
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
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorDotProps,
} from "components/StatusIndicator/StatusIndicator";
import type { FC } from "react";
import { getDisplayWorkspaceStatus } from "utils/workspace";

export const useTemplateFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	return useFilterMenu({
		onChange,
		value,
		id: "template",
		getSelectedOption: async () => {
			// Show all templates including deprecated
			const templates = await API.getTemplates();
			const template = templates.find((template) => template.name === value);
			if (template) {
				return {
					label: template.display_name || template.name,
					value: template.name,
					startIcon: <TemplateAvatar template={template} />,
				} satisfies SelectFilterOption;
			}
			return null;
		},
		getOptions: async (query) => {
			// Show all templates including deprecated
			const templates = await API.getTemplates();
			const filteredTemplates = templates.filter(
				(template) =>
					template.name.toLowerCase().includes(query.toLowerCase()) ||
					template.display_name.toLowerCase().includes(query.toLowerCase()),
			);
			return filteredTemplates.map((template) => ({
				label: template.display_name || template.name,
				value: template.name,
				startIcon: <TemplateAvatar template={template} />,
			}));
		},
	});
};

const TemplateAvatar: FC<{ template: Template }> = ({ template }) => {
	return (
		<Avatar
			size="sm"
			variant="icon"
			src={template.icon}
			fallback={template.display_name || template.name}
		/>
	);
};

export type TemplateFilterMenu = ReturnType<typeof useTemplateFilterMenu>;

type TemplateMenuProps = Readonly<{
	width?: number;
	menu: TemplateFilterMenu;
}>;

export const TemplateMenu: FC<TemplateMenuProps> = ({ width, menu }) => {
	return (
		<SelectFilter
			width={width}
			label="Select a template"
			emptyText="No templates found"
			placeholder="All templates"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
			selectFilterSearch={
				<SelectFilterSearch
					inputProps={{ "aria-label": "Search template" }}
					placeholder="Search template..."
					value={menu.query}
					onChange={menu.setQuery}
				/>
			}
		/>
	);
};

/** Status Filter Menu */

export const useStatusFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	const statusesToFilter: WorkspaceStatus[] = [
		"running",
		"stopped",
		"failed",
		"pending",
	];
	const statusOptions = statusesToFilter.map((status) => {
		const display = getDisplayWorkspaceStatus(status);
		return {
			label: display.text,
			value: status,
			startIcon: (
				<StatusIndicatorDot variant={getStatusIndicatorVariant(status)} />
			),
		};
	});
	return useFilterMenu({
		onChange,
		value,
		id: "status",
		getSelectedOption: async () =>
			statusOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => statusOptions,
	});
};

export type StatusFilterMenu = ReturnType<typeof useStatusFilterMenu>;

type StatusMenuProps = Readonly<{
	width?: number;
	menu: StatusFilterMenu;
}>;

export const StatusMenu: FC<StatusMenuProps> = ({ width, menu }) => {
	return (
		<SelectFilter
			width={width}
			placeholder="All statuses"
			label="Select a status"
			options={menu.searchOptions}
			selectedOption={menu.selectedOption ?? undefined}
			onSelect={menu.selectOption}
		/>
	);
};

export const getStatusIndicatorVariant = (
	status: WorkspaceStatus,
): StatusIndicatorDotProps["variant"] => {
	switch (status) {
		case "running":
			return "success";
		case "starting":
		case "pending":
			return "pending";
		case undefined:
		case "canceling":
		case "canceled":
		case "stopping":
		case "stopped":
			return "inactive";
		case "deleting":
		case "deleted":
			return "warning";
		case "failed":
			return "failed";
	}
};
