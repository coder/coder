import { MissingBuildParameters } from "api/api";
import { updateWorkspace } from "api/queries/workspaces";
import type {
	TemplateVersion,
	Workspace,
	WorkspaceBuild,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import { UpdateBuildParametersDialog } from "pages/WorkspacePage/UpdateBuildParametersDialog";
import { type FC, useState } from "react";
import { useMutation, useQueryClient } from "react-query";

type UseWorkspaceUpdateOptions = {
	workspace: Workspace;
	latestVersion: TemplateVersion | undefined;
	onSuccess?: (build: WorkspaceBuild) => void;
	onError?: (error: unknown) => void;
};

type UseWorkspaceUpdateResult = {
	update: () => void;
	isUpdating: boolean;
	dialogs: {
		updateConfirmation: UpdateConfirmationDialogProps;
		missingBuildParameters: MissingBuildParametersDialogProps;
	};
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

	const confirmUpdate = (buildParameters: WorkspaceBuildParameter[] = []) => {
		updateWorkspaceMutation.mutate(buildParameters);
		setIsConfirmingUpdate(false);
	};

	return {
		update,
		isUpdating: updateWorkspaceMutation.isLoading,
		dialogs: {
			updateConfirmation: {
				open: isConfirmingUpdate,
				onClose: () => setIsConfirmingUpdate(false),
				onConfirm: () => confirmUpdate(),
				latestVersion,
			},
			missingBuildParameters: {
				error: updateWorkspaceMutation.error,
				onClose: () => {
					updateWorkspaceMutation.reset();
				},
				onUpdate: (buildParameters: WorkspaceBuildParameter[]) => {
					if (updateWorkspaceMutation.error instanceof MissingBuildParameters) {
						confirmUpdate(buildParameters);
					}
				},
			},
		},
	};
};

type WorkspaceUpdateDialogsProps = {
	updateConfirmation: UpdateConfirmationDialogProps;
	missingBuildParameters: MissingBuildParametersDialogProps;
};

export const WorkspaceUpdateDialogs: FC<WorkspaceUpdateDialogsProps> = ({
	updateConfirmation,
	missingBuildParameters,
}) => {
	return (
		<>
			<UpdateConfirmationDialog {...updateConfirmation} />
			<MissingBuildParametersDialog {...missingBuildParameters} />
		</>
	);
};

type UpdateConfirmationDialogProps = {
	open: boolean;
	onClose: () => void;
	onConfirm: () => void;
	latestVersion?: TemplateVersion;
};

const UpdateConfirmationDialog: FC<UpdateConfirmationDialogProps> = ({
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

type MissingBuildParametersDialogProps = {
	error: unknown;
	onClose: () => void;
	onUpdate: (buildParameters: WorkspaceBuildParameter[]) => void;
};

const MissingBuildParametersDialog: FC<MissingBuildParametersDialogProps> = ({
	error,
	...dialogProps
}) => {
	return (
		<UpdateBuildParametersDialog
			missedParameters={
				error instanceof MissingBuildParameters ? error.parameters : []
			}
			open={error instanceof MissingBuildParameters}
			{...dialogProps}
		/>
	);
};
