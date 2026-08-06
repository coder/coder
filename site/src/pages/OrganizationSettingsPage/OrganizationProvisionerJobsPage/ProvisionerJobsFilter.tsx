import type { FC } from "react";
import { API } from "#/api/api";
import type { ProvisionerJobStatus, Template } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { ComboboxInput } from "#/components/Combobox/Combobox";
import {
	Filter,
	MenuSkeleton,
	type useFilter,
} from "#/components/Filter/Filter";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "#/components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "#/components/Filter/SelectFilter";
import {
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "#/components/StatusIndicator/StatusIndicator";
import { docs } from "#/utils/docs";

const variantByStatus: Record<
	ProvisionerJobStatus,
	StatusIndicatorProps["variant"]
> = {
	succeeded: "success",
	failed: "failed",
	pending: "pending",
	running: "pending",
	canceling: "pending",
	canceled: "inactive",
	unknown: "inactive",
};

const statusOptions: SelectFilterOption[] = (
	[
		"succeeded",
		"pending",
		"running",
		"canceling",
		"canceled",
		"failed",
		"unknown",
	] as const
).map((status) => ({
	label: status.charAt(0).toUpperCase() + status.slice(1),
	value: status,
	startIcon: <StatusIndicatorDot variant={variantByStatus[status]} />,
}));

const typeOptions: SelectFilterOption[] = [
	{ value: "workspace_build", label: "Workspace build" },
	{ value: "template_version_import", label: "Template import" },
	{ value: "template_version_dry_run", label: "Template dry-run" },
];

const templateOption = (template: Template): SelectFilterOption => ({
	label: template.display_name || template.name,
	value: template.name,
	startIcon: (
		<Avatar
			size="sm"
			variant="icon"
			src={template.icon}
			fallback={template.display_name || template.name}
		/>
	),
});

export const useProvisionerJobStatusFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	return useFilterMenu({
		id: "provisioner-job-status",
		value,
		onChange,
		getSelectedOption: async () =>
			statusOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => statusOptions,
	});
};

type ProvisionerJobStatusFilterMenu = ReturnType<
	typeof useProvisionerJobStatusFilterMenu
>;

export const useProvisionerJobTypeFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	return useFilterMenu({
		id: "provisioner-job-type",
		value,
		onChange,
		getSelectedOption: async () =>
			typeOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => typeOptions,
	});
};

type ProvisionerJobTypeFilterMenu = ReturnType<
	typeof useProvisionerJobTypeFilterMenu
>;

export const useProvisionerJobTemplateFilterMenu = ({
	organizationId,
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange"> & {
	organizationId: string | undefined;
}) => {
	return useFilterMenu({
		id: "provisioner-job-template",
		value,
		onChange,
		enabled: Boolean(organizationId),
		getSelectedOption: async () => {
			if (!organizationId || !value) {
				return null;
			}
			const templates = await API.getTemplatesByOrganization(organizationId);
			const match = templates.find((template) => template.name === value);
			return match ? templateOption(match) : null;
		},
		getOptions: async (query) => {
			if (!organizationId) {
				return [];
			}
			const templates = await API.getTemplatesByOrganization(organizationId);
			const filtered = templates.filter(
				(template) =>
					template.name.toLowerCase().includes(query.toLowerCase()) ||
					template.display_name.toLowerCase().includes(query.toLowerCase()),
			);
			return filtered.map(templateOption);
		},
	});
};

type ProvisionerJobTemplateFilterMenu = ReturnType<
	typeof useProvisionerJobTemplateFilterMenu
>;

type ProvisionerJobsFilterProps = {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		status: ProvisionerJobStatusFilterMenu;
		type: ProvisionerJobTypeFilterMenu;
		template: ProvisionerJobTemplateFilterMenu;
	};
};

export const ProvisionerJobsFilter: FC<ProvisionerJobsFilterProps> = ({
	filter,
	error,
	menus,
}) => {
	return (
		<Filter
			filter={filter}
			error={error}
			isLoading={
				menus.status.isInitializing ||
				menus.type.isInitializing ||
				menus.template.isInitializing
			}
			optionsSkeleton={
				<>
					<MenuSkeleton />
					<MenuSkeleton />
					<MenuSkeleton />
				</>
			}
			learnMoreLink={docs("/admin/provisioners")}
			presets={[
				{ name: "All jobs", query: "" },
				{ name: "Running", query: "status:running" },
				{ name: "Failed", query: "status:failed" },
				{ name: "Pending", query: "status:pending" },
				{ name: "Workspace builds", query: "type:workspace_build" },
				{
					name: "Template imports",
					query: "type:template_version_import",
				},
			]}
			options={
				<>
					<SelectFilter
						label="Select a status"
						placeholder="All statuses"
						options={menus.status.searchOptions}
						selectedOption={menus.status.selectedOption ?? undefined}
						onSelect={menus.status.selectOption}
					/>
					<SelectFilter
						label="Select a type"
						placeholder="All types"
						options={menus.type.searchOptions}
						selectedOption={menus.type.selectedOption ?? undefined}
						onSelect={menus.type.selectOption}
					/>
					<SelectFilter
						label="Select a template"
						placeholder="All templates"
						emptyText="No templates found"
						options={menus.template.searchOptions}
						selectedOption={menus.template.selectedOption ?? undefined}
						onSelect={menus.template.selectOption}
						selectFilterSearch={
							<ComboboxInput
								aria-label="Search template"
								placeholder="Search template..."
								value={menus.template.query}
								onValueChange={menus.template.setQuery}
							/>
						}
					/>
				</>
			}
		/>
	);
};
