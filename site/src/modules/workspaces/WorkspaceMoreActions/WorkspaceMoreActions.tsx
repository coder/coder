import { MissingBuildParameters } from "api/api";
import {
	changeVersion,
	deleteWorkspace,
	workspacePermissions,
} from "api/queries/workspaces";
import type { Workspace } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import {
	EllipsisVertical,
	SettingsIcon,
	HistoryIcon,
	TrashIcon,
	CopyIcon,
	DownloadIcon,
} from "lucide-react";
import { UpdateBuildParametersDialog } from "./UpdateBuildParametersDialog";
import { DownloadLogsDialog } from "./DownloadLogsDialog";
import { useState, type FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink } from "react-router-dom";
import { ChangeWorkspaceVersionDialog } from "./ChangeWorkspaceVersionDialog";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";
import type { WorkspacePermissions } from "../permissions";
import { useWorkspaceDuplication } from "./useWorkspaceDuplication";

type WorkspaceMoreActionsProps = {
	workspace: Workspace;
	disabled?: boolean;
};

export const WorkspaceMoreActions: FC<WorkspaceMoreActionsProps> = ({
	workspace,
	disabled,
}) => {
	const queryClient = useQueryClient();

	// Permissions
	const { data: permissions } = useQuery(workspacePermissions(workspace));

	// Download logs
	const [isDownloadDialogOpen, setIsDownloadDialogOpen] = useState(false);

	// Change version
	const [changeVersionDialogOpen, setChangeVersionDialogOpen] = useState(false);
	const changeVersionMutation = useMutation(
		changeVersion(workspace, queryClient),
	);

	// Delete
	const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
	const deleteWorkspaceMutation = useMutation(
		deleteWorkspace(workspace, queryClient),
	);

	// Duplicate
	const { duplicateWorkspace, isDuplicationReady } =
		useWorkspaceDuplication(workspace);

	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						size="icon-lg"
						variant="subtle"
						data-testid="workspace-options-button"
						aria-controls="workspace-options"
						disabled={disabled}
					>
						<EllipsisVertical aria-hidden="true" />
						<span className="sr-only">Workspace actions</span>
					</Button>
				</DropdownMenuTrigger>

				<DropdownMenuContent id="workspace-options" align="end">
					<DropdownMenuItem asChild>
						<RouterLink to="settings">
							<SettingsIcon />
							Settings
						</RouterLink>
					</DropdownMenuItem>

					{permissions?.updateWorkspaceVersion && (
						<DropdownMenuItem
							onClick={() => {
								setChangeVersionDialogOpen(true);
							}}
						>
							<HistoryIcon />
							Change version&hellip;
						</DropdownMenuItem>
					)}

					<DropdownMenuItem
						onClick={duplicateWorkspace}
						disabled={!isDuplicationReady}
					>
						<CopyIcon />
						Duplicate&hellip;
					</DropdownMenuItem>

					<DropdownMenuItem onClick={() => setIsDownloadDialogOpen(true)}>
						<DownloadIcon />
						Download logs&hellip;
					</DropdownMenuItem>

					<DropdownMenuSeparator />

					<DropdownMenuItem
						className="text-content-destructive focus:text-content-destructive"
						onClick={() => {
							setIsConfirmingDelete(true);
						}}
						data-testid="delete-button"
					>
						<TrashIcon />
						Delete&hellip;
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>

			<DownloadLogsDialog
				workspace={workspace}
				open={isDownloadDialogOpen}
				onClose={() => setIsDownloadDialogOpen(false)}
			/>

			<UpdateBuildParametersDialog
				missedParameters={
					isMissingBuildParameters(changeVersionMutation.error)
						? changeVersionMutation.error.parameters
						: []
				}
				open={isMissingBuildParameters(changeVersionMutation.error)}
				onClose={() => {
					changeVersionMutation.reset();
				}}
				onUpdate={(buildParameters) => {
					if (isMissingBuildParameters(changeVersionMutation.error)) {
						changeVersionMutation.mutate({
							versionId: changeVersionMutation.error.versionId,
							buildParameters,
						});
					}
				}}
			/>

			<ChangeWorkspaceVersionDialog
				workspace={workspace}
				open={changeVersionDialogOpen}
				onClose={() => {
					setChangeVersionDialogOpen(false);
				}}
				onConfirm={(version) => {
					setChangeVersionDialogOpen(false);
					changeVersionMutation.mutate({ versionId: version.id });
				}}
			/>

			<WorkspaceDeleteDialog
				workspace={workspace}
				canDeleteFailedWorkspace={!!permissions?.deleteFailedWorkspace}
				isOpen={isConfirmingDelete}
				onCancel={() => {
					setIsConfirmingDelete(false);
				}}
				onConfirm={(orphan) => {
					deleteWorkspaceMutation.mutate({ orphan });
					setIsConfirmingDelete(false);
				}}
			/>
		</>
	);
};

const isMissingBuildParameters = (e: unknown): e is MissingBuildParameters => {
	return Boolean(e && e instanceof MissingBuildParameters);
};
