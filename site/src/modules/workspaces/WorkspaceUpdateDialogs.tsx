import { type FC, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link } from "react-router";
import { ParameterValidationError } from "#/api/api";
import { updateWorkspace } from "#/api/queries/workspaces";
import type {
	TemplateVersion,
	Workspace,
	WorkspaceBuild,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { MemoizedInlineMarkdown } from "#/components/Markdown/InlineMarkdown";

type UseWorkspaceUpdateOptions = {
	workspace: Workspace;
	latestVersion: TemplateVersion | undefined;
	onSuccess?: (build: WorkspaceBuild) => void;
	onError?: (error: unknown) => void;
};

type UseWorkspaceUpdateResult = {
	update: () => void;
	isUpdating: boolean;
	dialogProps: WorkspaceUpdateDialogsProps;
};

export const useWorkspaceUpdate = ({
	workspace,
	latestVersion,
	onSuccess,
	onError,
}: UseWorkspaceUpdateOptions): UseWorkspaceUpdateResult => {
	const queryClient = useQueryClient();
	const [isConfirmingUpdate, setIsConfirmingUpdate] = useState(false);

	const updateWorkspaceOptions = updateWorkspace(workspace, queryClient);
	const updateWorkspaceMutation = useMutation({
		...updateWorkspaceOptions,
		onSuccess: (build: WorkspaceBuild) => {
			updateWorkspaceOptions.onSuccess(build);
			onSuccess?.(build);
		},
		onError,
	});

	const update = () => {
		setIsConfirmingUpdate(true);
	};

	const confirmUpdate = () => {
		updateWorkspaceMutation.mutate({
			buildParameters: [],
		});
		setIsConfirmingUpdate(false);
	};

	return {
		update,
		isUpdating: updateWorkspaceMutation.isPending,
		dialogProps: {
			confirmUpdateDialogProps: {
				open: isConfirmingUpdate,
				onClose: () => setIsConfirmingUpdate(false),
				onConfirm: () => confirmUpdate(),
				latestVersion,
			},
			updateBuildParametersDialogProps:
				updateWorkspaceMutation.error instanceof ParameterValidationError
					? {
							workspace,
							error: updateWorkspaceMutation.error,
							onClose: () => {
								updateWorkspaceMutation.reset();
							},
						}
					: undefined,
		},
	};
};

type WorkspaceUpdateDialogsProps = {
	confirmUpdateDialogProps: ConfirmUpdateDialogProps;
	updateBuildParametersDialogProps?: UpdateBuildParametersDialogProps;
};

export const WorkspaceUpdateDialogs: FC<WorkspaceUpdateDialogsProps> = ({
	confirmUpdateDialogProps,
	updateBuildParametersDialogProps,
}) => {
	return (
		<>
			<ConfirmUpdateDialog {...confirmUpdateDialogProps} />
			{updateBuildParametersDialogProps && (
				<UpdateBuildParametersDialog {...updateBuildParametersDialogProps} />
			)}
		</>
	);
};

type ConfirmUpdateDialogProps = {
	open: boolean;
	onClose: () => void;
	onConfirm: () => void;
	latestVersion?: TemplateVersion;
};

const ConfirmUpdateDialog: FC<ConfirmUpdateDialogProps> = ({
	latestVersion,
	...dialogProps
}) => {
	return (
		<ConfirmDialog
			{...dialogProps}
			hideCancel={false}
			title="Update workspace?"
			confirmText="Update"
			description={
				<div className="flex flex-col gap-2">
					<p>
						Updating your workspace will start the workspace on the latest
						template version. This can{" "}
						<strong>delete non-persistent data</strong>.
					</p>
					{latestVersion?.message && (
						<MemoizedInlineMarkdown allowedElements={["ol", "ul", "li"]}>
							{latestVersion.message}
						</MemoizedInlineMarkdown>
					)}
				</div>
			}
		/>
	);
};

type UpdateBuildParametersDialogProps = {
	workspace: Workspace;
	error: ParameterValidationError;
	onClose: () => void;
};

export const UpdateBuildParametersDialog: FC<
	UpdateBuildParametersDialogProps
> = ({ workspace, error, onClose }) => {
	const templateVersionId = error.versionId;
	const validations = error.validations;

	return (
		<Dialog open onOpenChange={() => onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Update workspace parameters</DialogTitle>
					<DialogDescription>
						This workspace has{" "}
						<strong className="text-content-primary">
							{validations.length} parameter
							{validations.length === 1 ? "" : "s"}
						</strong>{" "}
						that must be configured to complete the update.
					</DialogDescription>
					<DialogDescription>
						Would you like to go to the workspace parameters page to review and
						update these parameters before continuing?
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<Button onClick={onClose} variant="outline">
						Cancel
					</Button>
					<Button asChild>
						<Link
							to={`/@${workspace.owner_name}/${workspace.name}/settings/parameters?templateVersionId=${templateVersionId}`}
						>
							Go to workspace parameters
						</Link>
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
