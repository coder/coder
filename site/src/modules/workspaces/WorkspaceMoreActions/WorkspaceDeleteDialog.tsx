import type {
	CreateWorkspaceBuildRequest,
	Workspace,
} from "api/typesGenerated";
import { Checkbox } from "components/Checkbox/Checkbox";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import dayjs from "dayjs";
import { type FC, type FormEvent, useId, useState } from "react";
import { cn } from "utils/cn";
import { docs } from "utils/docs";

interface WorkspaceDeleteDialogProps {
	workspace: Workspace;
	canDeleteFailedWorkspace: boolean;
	isOpen: boolean;
	onCancel: () => void;
	onConfirm: (arg: CreateWorkspaceBuildRequest["orphan"]) => void;
}

export const WorkspaceDeleteDialog: FC<WorkspaceDeleteDialogProps> = ({
	workspace,
	canDeleteFailedWorkspace,
	isOpen,
	onCancel,
	onConfirm,
}) => {
	const hookId = useId();
	const [userConfirmationText, setUserConfirmationText] = useState("");
	const [orphanWorkspace, setOrphanWorkspace] =
		useState<CreateWorkspaceBuildRequest["orphan"]>(false);
	const [isFocused, setIsFocused] = useState(false);

	const deletionConfirmed = workspace.name === userConfirmationText;
	const onSubmit = (event: FormEvent) => {
		event.preventDefault();
		if (deletionConfirmed) {
			onConfirm(orphanWorkspace);
		}
	};

	const hasError = !deletionConfirmed && userConfirmationText.length > 0;
	const displayErrorMessage = hasError && !isFocused;
	// Orphaning is sort of a "last resort" that should really only
	// be used under the following circumstances:
	// a) Terraform is failing to apply while deleting, which
	//    usually means that builds are failing as well.
	// b) No provisioner is available to delete the workspace, which will
	//    cause the job to remain in the "pending" state indefinitely.
	//    The assumption here is that an admin will cancel the job, in which
	//    case we want to allow them to perform an orphan-delete.
	const canOrphan =
		canDeleteFailedWorkspace &&
		(workspace.latest_build.status === "failed" ||
			workspace.latest_build.status === "canceled");

	const hasTask = !!workspace.task_id;

	return (
		<ConfirmDialog
			type="delete"
			hideCancel={false}
			open={isOpen}
			title="Delete Workspace"
			onConfirm={() => onConfirm(orphanWorkspace)}
			onClose={onCancel}
			disabled={!deletionConfirmed}
			description={
				<>
					<div className="flex justify-between rounded-md p-4 mb-5 leading-snug border border-solid border-border">
						<div>
							<p className="text-base font-semibold text-content-primary m-0">
								{workspace.name}
							</p>
							<p className="text-xs text-content-secondary m-0">workspace</p>
						</div>
						<div className="text-right">
							<p className="text-xs font-medium text-content-primary m-0">
								{dayjs(workspace.created_at).fromNow()}
							</p>
							<p className="text-xs text-content-secondary m-0">created</p>
						</div>
					</div>

					<p>Deleting this workspace is irreversible!</p>
					<p>
						Type &ldquo;<strong>{workspace.name}</strong>&rdquo; below to
						confirm:
					</p>

					<form onSubmit={onSubmit}>
						<div className="mt-8 flex flex-col gap-2">
							<Label htmlFor={`${hookId}-confirm`}>Workspace name</Label>
							<Input
								autoFocus
								name="confirmation"
								autoComplete="off"
								id={`${hookId}-confirm`}
								placeholder={workspace.name}
								value={userConfirmationText}
								onChange={(event) =>
									setUserConfirmationText(event.target.value)
								}
								onFocus={() => setIsFocused(true)}
								onBlur={() => setIsFocused(false)}
								aria-invalid={displayErrorMessage || undefined}
								data-testid="delete-dialog-name-confirmation"
							/>
							{displayErrorMessage && (
								<p className="text-xs text-content-destructive m-0">
									{userConfirmationText} does not match the name of this
									workspace
								</p>
							)}
						</div>
						{hasTask && (
							<div className="mt-6 flex bg-surface-destructive border border-solid border-border-destructive rounded-lg p-3 gap-2 leading-[18px]">
								<div className="flex flex-col">
									<p className="text-sm font-semibold text-content-destructive m-0">
										This workspace is related to a task
									</p>
									<span className="text-xs mt-1 block">
										Deleting this workspace will also delete{" "}
										<a
											href={`/tasks/${workspace.owner_name}/${workspace.task_id}`}
											className="text-content-link hover:underline"
										>
											this task
										</a>
										.
									</span>
								</div>
							</div>
						)}
						{canOrphan && (
							<div className="mt-6 flex bg-surface-destructive border border-solid border-border-destructive rounded-lg p-3 gap-2 leading-[18px]">
								<div className="flex flex-col items-start pt-0.5">
									<Checkbox
										id="orphan_resources"
										checked={orphanWorkspace || false}
										onCheckedChange={(checked) => {
											setOrphanWorkspace(checked === true);
										}}
										className={cn(
											"border-content-destructive",
											"data-[state=checked]:bg-content-destructive data-[state=checked]:text-white",
										)}
										data-testid="orphan-checkbox"
									/>
								</div>
								<div className="flex flex-col">
									<label
										htmlFor="orphan_resources"
										className="text-sm font-semibold text-content-destructive cursor-pointer"
									>
										Orphan Resources
									</label>
									<span className="text-xs mt-1 block">
										As a Template Admin, you may skip resource cleanup to delete
										a failed workspace. Resources such as volumes and virtual
										machines will not be destroyed.&nbsp;
										<a
											href={docs(
												"/user-guides/workspace-management#workspace-resources",
											)}
											target="_blank"
											rel="noreferrer"
											className="text-content-link hover:underline"
										>
											Learn more...
										</a>
									</span>
								</div>
							</div>
						)}
					</form>
				</>
			}
		/>
	);
};
