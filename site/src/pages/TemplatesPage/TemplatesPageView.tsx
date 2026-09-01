import { ArrowRightIcon, PlusIcon, TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import { hasError, isApiValidationError } from "#/api/errors";
import type {
	AuthorizationResponse,
	Template,
	TemplateExample,
} from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "#/components/Avatar/AvatarDataSkeleton";
import { Badge } from "#/components/Badge/Badge";
import { DeprecatedBadge } from "#/components/Badges/Badges";
import { Button } from "#/components/Button/Button";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverLink,
	HelpPopoverLinksGroup,
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { Link } from "#/components/Link/Link";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "#/components/TableLoader/TableLoader";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import type { WorkspacePermissions } from "#/modules/permissions/workspaces";
import { cn } from "#/utils/cn";
import { createDayString } from "#/utils/createDayString";
import { docs } from "#/utils/docs";
import {
	formatTemplateActiveDevelopersLabel,
	formatTemplateBuildTime,
} from "#/utils/templates";
import { EmptyTemplates } from "./EmptyTemplates";
import { type TemplateFilterState, TemplatesFilter } from "./TemplatesFilter";

const ClassicParameterFlowAlert: FC<{ templateCount: number }> = ({
	templateCount,
}) => {
	return (
		<Alert
			severity="warning"
			prominent
			className="mt-6"
			actions={
				<Link
					href={docs("/admin/templates/extending-templates/dynamic-parameters")}
					target="_blank"
					rel="noreferrer"
				>
					View docs
					<span className="sr-only"> (opens in new tab)</span>
				</Link>
			}
		>
			<AlertTitle>
				{templateCount === 1
					? "1 template still uses the classic parameter flow"
					: `${templateCount} templates still use the classic parameter flow`}
			</AlertTitle>
			<AlertDescription>
				The classic parameter flow is deprecated. Move these templates to
				dynamic parameters to get real-time validation, conditional parameters,
				and richer input types.
			</AlertDescription>
		</Alert>
	);
};

const TemplateHelpPopover: FC = () => {
	return (
		<HelpPopover>
			<HelpPopoverIconTrigger />
			<HelpPopoverContent>
				<HelpPopoverTitle>What is a template?</HelpPopoverTitle>
				<HelpPopoverText>
					With templates you can create a common configuration for your
					workspaces using Terraform.
				</HelpPopoverText>
				<HelpPopoverLinksGroup>
					<HelpPopoverLink href={docs("/admin/templates")}>
						Manage templates
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
			</HelpPopoverContent>
		</HelpPopover>
	);
};

interface TemplateActionsProps {
	template: Template;
	workspacePermissions: Record<string, WorkspacePermissions> | undefined;
	templatePageLink: string;
}

const TemplateActions: FC<TemplateActionsProps> = ({
	template,
	workspacePermissions,
	templatePageLink,
}) => {
	if (template.deleted) {
		return null;
	}

	if (template.deprecated) {
		return <DeprecatedBadge />;
	}

	if (
		!workspacePermissions?.[template.organization_id]?.createWorkspaceForUserID
	) {
		return null;
	}

	return (
		<Button
			asChild
			variant="outline"
			size="sm"
			className="transition-none group-hover:border-border-secondary"
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
	);
};

interface TemplateRowProps {
	canUpdateTemplate: boolean;
	showOrganizations: boolean;
	template: Template;
	workspacePermissions: Record<string, WorkspacePermissions> | undefined;
}

const TemplateRow: FC<TemplateRowProps> = ({
	canUpdateTemplate,
	showOrganizations,
	template,
	workspacePermissions,
}) => {
	const getLink = useLinks();
	const templatePageLink = getLink(
		linkToTemplate(template.organization_name, template.name),
	);
	const navigate = useNavigate();

	const developerCount = formatTemplateActiveDevelopersLabel(
		template.active_user_count,
	);

	const clickableRow = useClickableTableRow({
		onClick: () => navigate(templatePageLink),
	});

	return (
		<TableRow
			key={template.id}
			data-testid={`template-${template.id}`}
			{...clickableRow}
			className={cn("group", clickableRow.className)}
		>
			<TableCell>
				<AvatarData
					title={
						<span className="flex flex-row items-center gap-2">
							{template.display_name || template.name}
							{canUpdateTemplate && template.use_classic_parameter_flow && (
								<Badge variant="warning" size="sm">
									<TriangleAlertIcon aria-hidden="true" />
									Classic parameters
								</Badge>
							)}
						</span>
					}
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

			<TableCell className="text-content-secondary">
				{showOrganizations ? (
					<AvatarData
						title={template.organization_display_name}
						subtitle={`Used by ${developerCount}`}
						avatar={<Avatar variant="icon" src={template.organization_icon} />}
					/>
				) : (
					developerCount
				)}
			</TableCell>

			<TableCell className="text-content-secondary">
				{formatTemplateBuildTime(template.build_time_stats.start.P50)}
			</TableCell>

			<TableCell data-pixel="ignore" className="text-content-secondary">
				{createDayString(template.updated_at)}
			</TableCell>

			<TableCell className="whitespace-nowrap">
				<TemplateActions
					template={template}
					workspacePermissions={workspacePermissions}
					templatePageLink={templatePageLink}
				/>
			</TableCell>
		</TableRow>
	);
};

interface TemplatesPageViewProps {
	error?: unknown;
	filterState: TemplateFilterState;
	showOrganizations: boolean;
	canCreateTemplates: boolean;
	templateBuilderEnabled: boolean;
	examples: TemplateExample[] | undefined;
	templates: Template[] | undefined;
	templateUpdatePermissions: AuthorizationResponse;
	workspacePermissions: Record<string, WorkspacePermissions> | undefined;
}

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
	error,
	filterState,
	showOrganizations,
	canCreateTemplates,
	templateBuilderEnabled,
	examples,
	templates,
	templateUpdatePermissions,
	workspacePermissions,
}) => {
	const isLoading = !templates;
	const isEmpty = !isLoading && templates.length === 0;
	const classicParameterFlowTemplateCount =
		templates?.filter(
			(template) =>
				template.use_classic_parameter_flow &&
				templateUpdatePermissions[template.organization_id],
		).length ?? 0;
	const showClassicParameterFlow = classicParameterFlowTemplateCount > 0;

	return (
		<Margins className="pb-12">
			{showClassicParameterFlow && (
				<ClassicParameterFlowAlert
					templateCount={classicParameterFlowTemplateCount}
				/>
			)}

			<PageHeader
				actions={
					canCreateTemplates && (
						<Button asChild size="lg">
							<RouterLink
								to={
									templateBuilderEnabled
										? "/templates/new/builder"
										: "/starter-templates"
								}
							>
								<PlusIcon />
								New template
							</RouterLink>
						</Button>
					)
				}
			>
				<PageHeaderTitle>
					<div className="flex flex-row gap-2 items-center">
						Templates
						<TemplateHelpPopover />
					</div>
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
						<TableHead className="w-[35%]">Name</TableHead>
						<TableHead className="w-[15%]">
							{showOrganizations ? "Organization" : "Used by"}
						</TableHead>
						<TableHead className="w-[10%]">Build time</TableHead>
						<TableHead className="w-[15%]">Last updated</TableHead>
						<TableHead className="w-[1%]" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading ? (
						<TableLoader />
					) : isEmpty ? (
						<EmptyTemplates
							canCreateTemplates={canCreateTemplates}
							templateBuilderEnabled={templateBuilderEnabled}
							examples={examples ?? []}
							isUsingFilter={filterState.filter.used}
						/>
					) : (
						templates.map((template) => (
							<TemplateRow
								key={template.id}
								canUpdateTemplate={
									templateUpdatePermissions[template.organization_id] ?? false
								}
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
					<AvatarDataSkeleton />
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
