import type { Interpolation, Theme } from "@emotion/react";
import ArrowForwardOutlined from "@mui/icons-material/ArrowForwardOutlined";
import MuiButton from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { hasError, isApiValidationError } from "api/errors";
import type { Template, TemplateExample } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "components/Avatar/AvatarDataSkeleton";
import { DeprecatedBadge } from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import type { useFilter } from "components/Filter/Filter";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import { PlusIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import type { FC } from "react";
import { Link, useNavigate } from "react-router-dom";
import { createDayString } from "utils/createDayString";
import { docs } from "utils/docs";
import {
	formatTemplateActiveDevelopers,
	formatTemplateBuildTime,
} from "utils/templates";
import { CreateTemplateButton } from "./CreateTemplateButton";
import { EmptyTemplates } from "./EmptyTemplates";
import { TemplatesFilter } from "./TemplatesFilter";

export const Language = {
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
			<HelpTooltipTrigger />
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
}

const TemplateRow: FC<TemplateRowProps> = ({ showOrganizations, template }) => {
	const getLink = useLinks();
	const templatePageLink = getLink(
		linkToTemplate(template.organization_name, template.name),
	);
	const hasIcon = template.icon && template.icon !== "";
	const navigate = useNavigate();

	const { css: clickableCss, ...clickableRow } = useClickableTableRow({
		onClick: () => navigate(templatePageLink),
	});

	return (
		<TableRow
			key={template.id}
			data-testid={`template-${template.id}`}
			{...clickableRow}
			css={[clickableCss, styles.tableRow]}
		>
			<TableCell>
				<AvatarData
					title={template.display_name || template.name}
					subtitle={template.description}
					avatar={
						<Avatar
							variant="icon"
							src={template.icon}
							fallback={template.display_name || template.name}
						/>
					}
				/>
			</TableCell>

			<TableCell css={styles.secondary}>
				{showOrganizations ? (
					<Stack
						spacing={0}
						css={{
							width: "100%",
						}}
					>
						<span css={styles.cellPrimaryLine}>
							{template.organization_display_name}
						</span>
						<span css={styles.cellSecondaryLine}>
							Used by {Language.developerCount(template.active_user_count)}
						</span>
					</Stack>
				) : (
					Language.developerCount(template.active_user_count)
				)}
			</TableCell>

			<TableCell css={styles.secondary}>
				{formatTemplateBuildTime(template.build_time_stats.start.P50)}
			</TableCell>

			<TableCell data-chromatic="ignore" css={styles.secondary}>
				{createDayString(template.updated_at)}
			</TableCell>

			<TableCell css={styles.actionCell}>
				{template.deprecated ? (
					<DeprecatedBadge />
				) : (
					<MuiButton
						size="small"
						css={styles.actionButton}
						className="actionButton"
						startIcon={<ArrowForwardOutlined />}
						title={`Create a workspace using the ${template.display_name} template`}
						onClick={(e) => {
							e.stopPropagation();
							navigate(`${templatePageLink}/workspace`);
						}}
					>
						Create Workspace
					</MuiButton>
				)}
			</TableCell>
		</TableRow>
	);
};

export interface TemplatesPageViewProps {
	error?: unknown;
	filter: ReturnType<typeof useFilter>;
	showOrganizations: boolean;
	canCreateTemplates: boolean;
	examples: TemplateExample[] | undefined;
	templates: Template[] | undefined;
}

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
	error,
	filter,
	showOrganizations,
	canCreateTemplates,
	examples,
	templates,
}) => {
	const isLoading = !templates;
	const isEmpty = templates && templates.length === 0;
	const navigate = useNavigate();

	const createTemplateAction = showOrganizations ? (
		<Button asChild size="lg">
			<Link to="/starter-templates">
				<PlusIcon />
				New template
			</Link>
		</Button>
	) : (
		<CreateTemplateButton onNavigate={navigate} />
	);

	return (
		<Margins>
			<PageHeader actions={canCreateTemplates && createTemplateAction}>
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

			<TemplatesFilter filter={filter} error={error} />
			{/* Validation errors are shown on the filter, other errors are an alert box. */}
			{hasError(error) && !isApiValidationError(error) && (
				<ErrorAlert error={error} />
			)}

			<TableContainer>
				<Table>
					<TableHead>
						<TableRow>
							<TableCell width="35%">{Language.nameLabel}</TableCell>
							<TableCell width="15%">
								{showOrganizations ? "Organization" : Language.usedByLabel}
							</TableCell>
							<TableCell width="10%">{Language.buildTimeLabel}</TableCell>
							<TableCell width="15%">{Language.lastUpdatedLabel}</TableCell>
							<TableCell width="1%" />
						</TableRow>
					</TableHead>
					<TableBody>
						{isLoading && <TableLoader />}

						{isEmpty ? (
							<EmptyTemplates
								canCreateTemplates={canCreateTemplates}
								examples={examples ?? []}
							/>
						) : (
							templates?.map((template) => (
								<TemplateRow
									key={template.id}
									showOrganizations={showOrganizations}
									template={template}
								/>
							))
						)}
					</TableBody>
				</Table>
			</TableContainer>
		</Margins>
	);
};

const TableLoader: FC = () => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<div css={{ display: "flex", alignItems: "center", gap: 8 }}>
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

const styles = {
	templateIconWrapper: {
		// Same size then the avatar component
		width: 36,
		height: 36,
		padding: 2,

		"& img": {
			width: "100%",
		},
	},
	actionCell: {
		whiteSpace: "nowrap",
	},
	cellPrimaryLine: (theme) => ({
		color: theme.palette.text.primary,
		fontWeight: 600,
	}),
	cellSecondaryLine: (theme) => ({
		fontSize: 13,
		color: theme.palette.text.secondary,
		lineHeight: "150%",
	}),
	secondary: (theme) => ({
		color: theme.palette.text.secondary,
	}),
	tableRow: (theme) => ({
		"&:hover .actionButton": {
			color: theme.experimental.l2.hover.text,
			borderColor: theme.experimental.l2.hover.outline,
		},
	}),
	actionButton: (theme) => ({
		transition: "none",
		color: theme.palette.text.primary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
