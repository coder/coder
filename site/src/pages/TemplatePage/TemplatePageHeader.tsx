import AddIcon from "@mui/icons-material/AddOutlined";
import DeleteIcon from "@mui/icons-material/DeleteOutlined";
import EditIcon from "@mui/icons-material/EditOutlined";
import CopyIcon from "@mui/icons-material/FileCopyOutlined";
import SettingsIcon from "@mui/icons-material/SettingsOutlined";
import Button from "@mui/material/Button";
import Divider from "@mui/material/Divider";
import { workspaces } from "api/queries/workspaces";
import type {
	AuthorizationResponse,
	Template,
	TemplateVersion,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { Margins } from "components/Margins/Margins";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import {
	MoreMenu,
	MoreMenuContent,
	MoreMenuItem,
	MoreMenuTrigger,
	ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { linkToTemplate, useLinks } from "modules/navigation";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { useDeletionDialogState } from "./useDeletionDialogState";

type TemplateMenuProps = {
	organizationName: string;
	templateName: string;
	templateVersion: string;
	templateId: string;
	onDelete: () => void;
};

const TemplateMenu: FC<TemplateMenuProps> = ({
	organizationName,
	templateName,
	templateVersion,
	templateId,
	onDelete,
}) => {
	const dialogState = useDeletionDialogState(templateId, onDelete);
	const navigate = useNavigate();
	const getLink = useLinks();
	const queryText = `template:${templateName}`;
	const workspaceCountQuery = useQuery({
		...workspaces({ q: queryText }),
		select: (res) => res.count,
	});
	const safeToDeleteTemplate = workspaceCountQuery.data === 0;

	const templateLink = getLink(linkToTemplate(organizationName, templateName));

	return (
		<>
			<MoreMenu>
				<MoreMenuTrigger>
					<ThreeDotsButton />
				</MoreMenuTrigger>
				<MoreMenuContent>
					<MoreMenuItem
						onClick={() => {
							navigate(`${templateLink}/settings`);
						}}
					>
						<SettingsIcon />
						Settings
					</MoreMenuItem>

					<MoreMenuItem
						onClick={() => {
							navigate(`${templateLink}/versions/${templateVersion}/edit`);
						}}
					>
						<EditIcon />
						Edit files
					</MoreMenuItem>

					<MoreMenuItem
						onClick={() => {
							navigate(`/templates/new?fromTemplate=${templateId}`);
						}}
					>
						<CopyIcon />
						Duplicate&hellip;
					</MoreMenuItem>
					<Divider />
					<MoreMenuItem onClick={dialogState.openDeleteConfirmation} danger>
						<DeleteIcon />
						Delete&hellip;
					</MoreMenuItem>
				</MoreMenuContent>
			</MoreMenu>

			{safeToDeleteTemplate ? (
				<DeleteDialog
					isOpen={dialogState.isDeleteDialogOpen}
					onConfirm={dialogState.confirmDelete}
					onCancel={dialogState.cancelDeleteConfirmation}
					entity="template"
					name={templateName}
				/>
			) : (
				<ConfirmDialog
					type="info"
					title="Unable to delete"
					hideCancel={false}
					open={dialogState.isDeleteDialogOpen}
					onClose={dialogState.cancelDeleteConfirmation}
					confirmText="See workspaces"
					confirmLoading={workspaceCountQuery.status !== "success"}
					onConfirm={() => {
						navigate({
							pathname: "/workspaces",
							search: new URLSearchParams({ filter: queryText }).toString(),
						});
					}}
					description={
						<>
							{workspaceCountQuery.isSuccess && (
								<>
									This template is used by{" "}
									<strong>
										{workspaceCountQuery.data} workspace
										{workspaceCountQuery.data === 1 ? "" : "s"}
									</strong>
									. Please delete all related workspaces before deleting this
									template.
								</>
							)}

							{workspaceCountQuery.isLoading &&
								"Loading information about workspaces used by this template."}

							{workspaceCountQuery.isError &&
								"Unable to determine workspaces used by this template."}
						</>
					}
				/>
			)}
		</>
	);
};

export type TemplatePageHeaderProps = {
	template: Template;
	activeVersion: TemplateVersion;
	permissions: AuthorizationResponse;
	workspacePermissions: AuthorizationResponse;
	onDeleteTemplate: () => void;
};

export const TemplatePageHeader: FC<TemplatePageHeaderProps> = ({
	template,
	activeVersion,
	permissions,
	workspacePermissions,
	onDeleteTemplate,
}) => {
	const getLink = useLinks();
	const templateLink = getLink(
		linkToTemplate(template.organization_name, template.name),
	);

	return (
		<Margins>
			<PageHeader
				actions={
					<>
						{!template.deprecated && workspacePermissions.createWorkspace && (
							<Button
								variant="contained"
								startIcon={<AddIcon />}
								component={RouterLink}
								to={`${templateLink}/workspace`}
							>
								Create Workspace
							</Button>
						)}

						{permissions.canUpdateTemplate && (
							<TemplateMenu
								organizationName={template.organization_name}
								templateId={template.id}
								templateName={template.name}
								templateVersion={activeVersion.name}
								onDelete={onDeleteTemplate}
							/>
						)}
					</>
				}
			>
				<Stack direction="row">
					<Avatar
						size="lg"
						variant="icon"
						src={template.icon}
						fallback={template.name}
					/>

					<div>
						<Stack direction="row" alignItems="center" spacing={1}>
							<PageHeaderTitle>
								{template.display_name.length > 0
									? template.display_name
									: template.name}
							</PageHeaderTitle>
							{template.deprecated && <Pill type="warning">Deprecated</Pill>}
						</Stack>

						{template.deprecation_message !== "" ? (
							<PageHeaderSubtitle condensed>
								<MemoizedInlineMarkdown>
									{template.deprecation_message}
								</MemoizedInlineMarkdown>
							</PageHeaderSubtitle>
						) : (
							template.description !== "" && (
								<PageHeaderSubtitle condensed>
									{template.description}
								</PageHeaderSubtitle>
							)
						)}
					</div>
				</Stack>
			</PageHeader>
		</Margins>
	);
};
