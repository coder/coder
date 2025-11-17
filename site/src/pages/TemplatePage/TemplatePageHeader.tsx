import { API } from "api/api";
import { workspaces } from "api/queries/workspaces";
import type {
	AuthorizationResponse,
	Template,
	TemplateVersion,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button, Button as ShadcnButton } from "components/Button/Button";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Margins } from "components/Margins/Margins";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import {
	CopyIcon,
	DownloadIcon,
	EditIcon,
	EllipsisVertical,
	PlusIcon,
	SettingsIcon,
	TrashIcon,
} from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import type { WorkspacePermissions } from "modules/permissions/workspaces";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink, useNavigate } from "react-router";
import { TemplateStats } from "./TemplateStats";
import { useDeletionDialogState } from "./useDeletionDialogState";

type TemplateMenuProps = {
	organizationName: string;
	templateName: string;
	templateVersion: string;
	templateId: string;
	fileId: string;
	onDelete: () => void;
};

const TemplateMenu: FC<TemplateMenuProps> = ({
	organizationName,
	templateName,
	templateVersion,
	templateId,
	fileId,
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

	const handleExport = async (format?: "zip") => {
		try {
			const blob = await API.downloadTemplateVersion(fileId, format);
			const url = window.URL.createObjectURL(blob);
			const link = document.createElement("a");
			link.href = url;
			const extension = format === "zip" ? "zip" : "tar";
			link.download = `${templateName}-${templateVersion}.${extension}`;
			document.body.appendChild(link);
			link.click();
			document.body.removeChild(link);
			window.URL.revokeObjectURL(url);
		} catch (error) {
			console.error("Failed to export template:", error);
			// TODO: Show user-friendly error message
		}
	};

	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<ShadcnButton size="icon-lg" variant="subtle" aria-label="Open menu">
						<EllipsisVertical aria-hidden="true" />
						<span className="sr-only">Open menu</span>
					</ShadcnButton>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem
						onClick={() => navigate(`${templateLink}/settings`)}
					>
						<SettingsIcon className="size-icon-sm" />
						Settings
					</DropdownMenuItem>

					<DropdownMenuItem
						onClick={() =>
							navigate(`${templateLink}/versions/${templateVersion}/edit`)
						}
					>
						<EditIcon />
						Edit files
					</DropdownMenuItem>

					<DropdownMenuItem
						onClick={() =>
							navigate(`/templates/new?fromTemplate=${templateId}`)
						}
					>
						<CopyIcon className="size-icon-sm" />
						Duplicate&hellip;
					</DropdownMenuItem>

					<DropdownMenuItem onClick={() => handleExport()}>
						<DownloadIcon className="size-icon-sm" />
						Export as TAR
					</DropdownMenuItem>

					<DropdownMenuItem onClick={() => handleExport("zip")}>
						<DownloadIcon className="size-icon-sm" />
						Export as ZIP
					</DropdownMenuItem>
					<DropdownMenuSeparator />
					<DropdownMenuItem
						className="text-content-destructive focus:text-content-destructive"
						onClick={dialogState.openDeleteConfirmation}
					>
						<TrashIcon />
						Delete&hellip;
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>

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

type TemplatePageHeaderProps = {
	template: Template;
	activeVersion: TemplateVersion;
	permissions: AuthorizationResponse;
	workspacePermissions: WorkspacePermissions;
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
						{!template.deprecated &&
							!template.deleted &&
							workspacePermissions.createWorkspaceForUserID && (
								<Button asChild>
									<RouterLink to={`${templateLink}/workspace`}>
										<PlusIcon />
										Create Workspace
									</RouterLink>
								</Button>
							)}

						{permissions.canUpdateTemplate && (
							<TemplateMenu
								organizationName={template.organization_name}
								templateId={template.id}
								templateName={template.name}
								templateVersion={activeVersion.name}
								fileId={activeVersion.job.file_id}
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
			<div className="pb-8">
				<TemplateStats template={template} activeVersion={activeVersion} />
			</div>
		</Margins>
	);
};
