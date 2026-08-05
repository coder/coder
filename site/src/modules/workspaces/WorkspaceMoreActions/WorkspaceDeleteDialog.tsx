import dayjs from "dayjs";
import { type FC, type FormEvent, useId, useState } from "react";
import type {
	CreateWorkspaceBuildRequest,
	Workspace,
} from "#/api/typesGenerated";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

const warnBoxClassName =
	"mt-6 flex gap-2 rounded-lg border border-solid border-border-warning bg-surface-orange p-3 leading-snug text-content-warning";

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
	const confirmId = useId();
	const errorId = `${confirmId}-error`;
	const orphanId = `${confirmId}-orphan`;

	const [userConfirmationText, setUserConfirmationText] = useState("");
	const [orphanWorkspace, setOrphanWorkspace] =
		useState<CreateWorkspaceBuildRequest["orphan"]>(false);
	const [isFocused, setIsFocused] = useState(false);

	const deletionConfirmed = workspace.name === userConfirmationText;
	const hasError = !deletionConfirmed && userConfirmationText.length > 0;
	const displayErrorMessage = hasError && !isFocused;

	const onSubmit = (event: FormEvent) => {
		event.preventDefault();
		if (deletionConfirmed) {
			onConfirm(orphanWorkspace);
		}
	};

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

	const hasTask = Boolean(workspace.task_id);

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
					<div className="flex items-center justify-between rounded-md border border-solid border-border p-4 mb-5 leading-snug">
						<div>
							<p className="m-0 text-base font-semibold text-content-primary">
								{workspace.name}
							</p>
							<p className="m-0 text-xs text-content-secondary">workspace</p>
						</div>
						<div className="text-right">
							<p className="m-0 text-xs font-medium text-content-primary">
								{dayjs(workspace.created_at).fromNow()}
							</p>
							<p className="m-0 text-xs text-content-secondary">created</p>
						</div>
					</div>

					<p>Deleting this workspace is irreversible!</p>
					<p>
						Type &ldquo;<strong>{workspace.name}</strong>&rdquo; below to
						confirm:
					</p>

					<form className="mt-2 flex flex-col gap-2" onSubmit={onSubmit}>
						<Label htmlFor={confirmId}>Workspace name</Label>
						<Input
							id={confirmId}
							name="confirmation"
							autoComplete="off"
							autoFocus
							placeholder={workspace.name}
							value={userConfirmationText}
							onChange={(event) => setUserConfirmationText(event.target.value)}
							onFocus={() => setIsFocused(true)}
							onBlur={() => setIsFocused(false)}
							aria-invalid={displayErrorMessage}
							aria-describedby={displayErrorMessage ? errorId : undefined}
							data-testid="delete-dialog-name-confirmation"
						/>
						{displayErrorMessage && (
							<span id={errorId} className="text-xs text-content-destructive">
								{userConfirmationText} does not match the name of this workspace
							</span>
						)}

						{hasTask && (
							<div className={warnBoxClassName}>
								<div>
									<p className="m-0 text-sm font-semibold">
										This workspace is related to a task
									</p>
									<span className="mt-1 block text-xs text-content-secondary">
										Deleting this workspace will also delete{" "}
										<Link
											href={`/tasks/${workspace.owner_name}/${workspace.task_id}`}
											size="sm"
											showExternalIcon={false}
										>
											this task
										</Link>
										.
									</span>
								</div>
							</div>
						)}

						{canOrphan && (
							<div className={warnBoxClassName}>
								<label
									htmlFor={orphanId}
									className="flex items-start gap-2 cursor-pointer"
								>
									<Checkbox
										id={orphanId}
										name="orphan_resources"
										checked={orphanWorkspace}
										onCheckedChange={(checked) => {
											setOrphanWorkspace(checked === true);
										}}
										data-testid="orphan-checkbox"
										className="mt-0.5 border-content-warning hover:enabled:border-content-warning data-[state=checked]:bg-content-warning data-[state=checked]:border-content-warning data-[state=checked]:text-content-invert hover:data-[state=checked]:bg-content-warning hover:data-[state=checked]:border-content-warning"
									/>
									<span>
										<span className="block text-sm font-semibold">
											Orphan Resources
										</span>
										<span className="mt-1 block text-xs text-content-secondary">
											As a Template Admin, you may skip resource cleanup to
											delete a failed workspace. Resources such as volumes and
											virtual machines will not be destroyed.{" "}
											<Link
												href={docs(
													"/user-guides/workspace-management#workspace-resources",
												)}
												target="_blank"
												rel="noreferrer"
												size="sm"
											>
												Learn more
											</Link>
										</span>
									</span>
								</label>
							</div>
						)}
					</form>
				</>
			}
		/>
	);
};
