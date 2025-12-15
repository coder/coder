import Skeleton from "@mui/material/Skeleton";
import { hasError, isApiValidationError } from "api/errors";
import type { Template, TemplateExample } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "components/Avatar/AvatarDataSkeleton";
import { DeprecatedBadge } from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipIconTrigger,
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import { ArrowRightIcon, PlusIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import type { WorkspacePermissions } from "modules/permissions/workspaces";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import { cn } from "utils/cn";
import { createDayString } from "utils/createDayString";
import { docs } from "utils/docs";
import {
	formatTemplateActiveDevelopers,
	formatTemplateBuildTime,
} from "utils/templates";
import { EmptyTemplates } from "./EmptyTemplates";
import { TemplatesFilter } from "./TemplatesFilter";
import type { TemplateFilterState } from "./TemplatesPage";

const Language = {
	developerCount: (activeCount: number): string => {
		return `${formatTemplateActiveDevelopers(activeCount)} developer${
			activeCount !== 1 ? "s" : ""
		}`;
	},
	nameLabel: "Name",
	buildTimeLabel: "Build time",
	usedByLabel: "Used by",
	lastUpdatedLabel: "Last updated",
	templateTooltipTitle: "What is template?",
	templateTooltipText:
		"With templates you can create a common configuration for your workspaces using Terraform.",
	templateTooltipLink: "Manage templates",
};

const TemplateHelpTooltip: FC = () => {
	return (
		<HelpTooltip>
			<HelpTooltipIconTrigger />
			<HelpTooltipContent>
				<HelpTooltipTitle>{Language.templateTooltipTitle}</HelpTooltipTitle>
				<HelpTooltipText>{Language.templateTooltipText}</HelpTooltipText>
				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/admin/templates")}>
						{Language.templateTooltipLink}
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

interface TemplateRowProps {
	showOrganizations: boolean;
	template: Template;
	workspacePermissions: Record<string, WorkspacePermissions> | undefined;
}

const TemplateRow: FC<TemplateRowProps> = ({
	showOrganizations,
	template,
	workspacePermissions,
}) => {
	const getLink = useLinks();
	const templatePageLink = getLink(
		linkToTemplate(template.organization_name, template.name),
	);
	const navigate = useNavigate();

	const clickableRow = useClickableTableRow({
		onClick: () => navigate(templatePageLink),
	});

	return (
		<TableRow
			key={template.id}
			data-testid={`template-${template.id}`}
			{...clickableRow}
			className={classNames.tableRow}
		>
			<TableCell>
				<AvatarData
					title={template.display_name || template.name}
					subtitle={template.description}
					avatar={
						<Avatar
							size="lg"
							variant="icon"
							src={template.icon}
							fallback={template.display_name || template.name}
						/>
					}
				/>
			</TableCell>

			<TableCell className={classNames.secondary}>
				{showOrganizations ? (
					<Stack spacing={0} className="w-full">
						<span className={classNames.cellPrimaryLine}>
							{template.organization_display_name}
						</span>
						<span className={classNames.cellSecondaryLine}>
							Used by {Language.developerCount(template.active_user_count)}
						</span>
					</Stack>
				) : (
					Language.developerCount(template.active_user_count)
				)}
			</TableCell>

			<TableCell className={classNames.secondary}>
				{formatTemplateBuildTime(template.build_time_stats.start.P50)}
			</TableCell>

			<TableCell data-chromatic="ignore" className={classNames.secondary}>
				{createDayString(template.updated_at)}
			</TableCell>

			<TableCell className={classNames.actionCell}>
				{template.deprecated ? (
					<DeprecatedBadge />
				) : workspacePermissions?.[template.organization_id]
						?.createWorkspaceForUserID ? (
					<Button
						asChild
						variant="outline"
						size="sm"
						title={`Create a workspace using the ${template.display_name} template`}
						onClick={(e) => {
							e.stopPropagation();
						}}
					>
						<RouterLink to={`${templatePageLink}/workspace`}>
							<ArrowRightIcon />
							Create Workspace
						</RouterLink>
					</Button>
				) : null}
			</TableCell>
		</TableRow>
	);
};

interface TemplatesPageViewProps {
	error?: unknown;
	filterState: TemplateFilterState;
	showOrganizations: boolean;
	canCreateTemplates: boolean;
	examples: TemplateExample[] | undefined;
	templates: Template[] | undefined;
	workspacePermissions: Record<string, WorkspacePermissions> | undefined;
}

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
	error,
	filterState,
	showOrganizations,
	canCreateTemplates,
	examples,
	templates,
	workspacePermissions,
}) => {
	const isLoading = !templates;
	const isEmpty = templates && templates.length === 0;

	return (
		<Margins className="pb-12">
			<PageHeader
				actions={
					canCreateTemplates && (
						<Button asChild size="lg">
							<RouterLink to="/starter-templates">
								<PlusIcon />
								New template
							</RouterLink>
						</Button>
					)
				}
			>
				<PageHeaderTitle>
					<Stack spacing={1} direction="row" alignItems="center">
						Templates
						<TemplateHelpTooltip />
					</Stack>
				</PageHeaderTitle>
				<PageHeaderSubtitle>
					Select a template to create a workspace.
				</PageHeaderSubtitle>
			</PageHeader>

			<TemplatesFilter
				filter={filterState.filter}
				error={error}
				userMenu={filterState.menus.user}
			/>
			{/* Validation errors are shown on the filter, other errors are an alert box. */}
			{hasError(error) && !isApiValidationError(error) && (
				<ErrorAlert error={error} />
			)}

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead className="w-[35%]">{Language.nameLabel}</TableHead>
						<TableHead className="w-[15%]">
							{showOrganizations ? "Organization" : Language.usedByLabel}
						</TableHead>
						<TableHead className="w-[10%]">{Language.buildTimeLabel}</TableHead>
						<TableHead className="w-[15%]">
							{Language.lastUpdatedLabel}
						</TableHead>
						<TableHead className="w-[1%]" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading && <TableLoader />}

					{isEmpty ? (
						<EmptyTemplates
							canCreateTemplates={canCreateTemplates}
							examples={examples ?? []}
							isUsingFilter={filterState.filter.used}
						/>
					) : (
						templates?.map((template) => (
							<TemplateRow
								key={template.id}
								showOrganizations={showOrganizations}
								template={template}
								workspacePermissions={workspacePermissions}
							/>
						))
					)}
				</TableBody>
			</Table>
		</Margins>
	);
};

const TableLoader: FC = () => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<div className="flex items-center gap-2">
						<AvatarDataSkeleton />
					</div>
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};

const classNames = {
	// `size-9` is the same size then the avatar component
	templateIconWrapper: "size-9 p-0.5 [&_img]:w-full",
	actionCell: "whitespace-nowrap",
	cellPrimaryLine: "text-content-primary font-semibold",
	cellSecondaryLine: "text-[13px] text-content-secondary leading-normal",
	secondary: "text-content-secondary",
	tableRow: cn([
		"[&:hover_.actionButton]:text-content-link",
		"[&:hover_.actionButton]:border-content-link",
	]),
	actionButton: "transition-none text-content-primary",
};
