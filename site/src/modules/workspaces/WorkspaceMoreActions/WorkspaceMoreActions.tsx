import { MissingBuildParameters, ParameterValidationError } from "api/api";
import { type ApiError, getErrorMessage, isApiError } from "api/errors";
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
import { displayError } from "components/GlobalSnackbar/utils";
import {
	CopyIcon,
	DownloadIcon,
	EllipsisVertical,
	HistoryIcon,
	SettingsIcon,
	SquareIcon,
	TrashIcon,
} from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink } from "react-router";
import { WorkspaceErrorDialog } from "../ErrorDialog/WorkspaceErrorDialog";
import { ChangeWorkspaceVersionDialog } from "./ChangeWorkspaceVersionDialog";
import { DownloadLogsDialog } from "./DownloadLogsDialog";
import { UpdateBuildParametersDialog } from "./UpdateBuildParametersDialog";
import { UpdateBuildParametersDialogExperimental } from "./UpdateBuildParametersDialogExperimental";
import { useWorkspaceDuplication } from "./useWorkspaceDuplication";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";

type WorkspaceMoreActionsProps = {
	workspace: Workspace;
	disabled: boolean;
	onStop?: () => void;
	isStopping?: boolean;
};

export const WorkspaceMoreActions: FC<WorkspaceMoreActionsProps> = ({
	workspace,
	disabled,
	onStop,
	isStopping,
}) => {
	const queryClient = useQueryClient();

	const [workspaceErrorDialog, setWorkspaceErrorDialog] = useState<{
		open: boolean;
		error?: ApiError;
	}>({ open: false });

	// Permissions
	const { data: permissions } = useQuery(workspacePermissions(workspace));

	// Download logs
	const [isDownloadDialogOpen, setIsDownloadDialogOpen] = useState(false);

	// Change version
	const [changeVersionDialogOpen, setChangeVersionDialogOpen] = useState(false);
	const changeVersionMutation = useMutation(
		changeVersion(
			workspace,
			queryClient,
			!workspace.template_use_classic_parameter_flow,
		),
	);

	const handleError = (error: unknown) => {
		if (isApiError(error) && error.code === "ERR_BAD_REQUEST") {
			setWorkspaceErrorDialog({
				open: true,
				error: error,
			});
		} else {
			displayError(getErrorMessage(error, "Failed to delete workspace."));
		}
	};

	// Delete
	const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
	const deleteWorkspaceMutation = useMutation({
		...deleteWorkspace(workspace, queryClient),
		onError: (error: unknown) => {
			handleError(error);
		},
	});

	// Duplicate
	const { duplicateWorkspace, isDuplicationReady } =
		useWorkspaceDuplication(workspace);

	// Since the workspace state is not updated immediately after the mutation, we
	// need to be sure the menu is closed when the action gets disabled.
	// Reference: https://github.com/coder/coder/pull/17775#discussion_r2087273706
	const [open, setOpen] = useState(false);
	useEffect(() => {
		setOpen((open) => (disabled ? false : open));
	});

	return (
		<>
			<DropdownMenu open={open} onOpenChange={setOpen}>
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
					{onStop && (
						<DropdownMenuItem onClick={onStop} disabled={isStopping}>
							<SquareIcon />
							Stop&hellip;
						</DropdownMenuItem>
					)}

					<DropdownMenuItem asChild>
						<RouterLink
							to={`/@${workspace.owner_name}/${workspace.name}/settings`}
						>
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

			{workspace.template_use_classic_parameter_flow ? (
				<UpdateBuildParametersDialog
					missedParameters={
						changeVersionMutation.error instanceof MissingBuildParameters
							? changeVersionMutation.error.parameters
							: []
					}
					open={changeVersionMutation.error instanceof MissingBuildParameters}
					onClose={() => {
						changeVersionMutation.reset();
					}}
					onUpdate={(buildParameters) => {
						if (changeVersionMutation.error instanceof MissingBuildParameters) {
							changeVersionMutation.mutate({
								versionId: changeVersionMutation.error.versionId,
								buildParameters,
							});
						}
					}}
				/>
			) : (
				<UpdateBuildParametersDialogExperimental
					validations={
						changeVersionMutation.error instanceof ParameterValidationError
							? changeVersionMutation.error.validations
							: []
					}
					open={changeVersionMutation.error instanceof ParameterValidationError}
					onClose={() => {
						changeVersionMutation.reset();
					}}
					workspaceOwnerName={workspace.owner_name}
					workspaceName={workspace.name}
					templateVersionId={
						changeVersionMutation.error instanceof ParameterValidationError
							? changeVersionMutation.error.versionId
							: undefined
					}
				/>
			)}

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

			<WorkspaceErrorDialog
				open={workspaceErrorDialog.open}
				error={workspaceErrorDialog.error}
				onClose={() => setWorkspaceErrorDialog({ open: false })}
				showDetail={workspace.template_use_classic_parameter_flow}
				workspaceOwner={workspace.owner_name}
				workspaceName={workspace.name}
				templateVersionId={workspace.latest_build.template_version_id}
				isDeleting={true}
			/>
		</>
	);
};
