import { useEffect, useId, useRef } from "react";
import type { ExternalToast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import type { ChatProject, Workspace } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import {
	DeleteConfirmationField,
	type useDeleteConfirmation,
} from "#/components/Dialog/DeleteDialog/DeleteConfirmationField";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { persistentToast, toast } from "#/components/Toaster/toast";
import type {
	ProjectChatWorkspace,
	ProjectWorkspaceAction,
	ProjectWorkspaceActionResult,
} from "../../../utils/agentWorkspaceUtils";
import { RetryButton } from "../../RetryButton";

/** Counts a noun, choosing its singular or plural form. */
export const plural = (count: number, singular: string, pluralForm: string) =>
	`${count} ${count === 1 ? singular : pluralForm}`;

type ProjectWorkspacesSectionProps = {
	readonly project: ChatProject;
	readonly workspaces: readonly ProjectChatWorkspace[] | undefined;
	readonly error: unknown;
	readonly isRetrying: boolean;
	readonly onRetry: () => void;
	readonly action: ProjectWorkspaceAction;
	readonly onActionChange: (action: ProjectWorkspaceAction) => void;
	readonly confirmation: ReturnType<typeof useDeleteConfirmation>;
};

/**
 * The archive dialog's choice to leave, stop, or delete the workspaces behind
 * the user's own chats in the project.
 */
export const ProjectWorkspacesSection: React.FC<
	ProjectWorkspacesSectionProps
> = ({
	project,
	workspaces,
	error,
	isRetrying,
	onRetry,
	action,
	onActionChange,
	confirmation,
}) => {
	const radioGroupLabelId = useId();
	const retryButtonRef = useRef<HTMLButtonElement>(null);
	const hasError = Boolean(error);
	// A failed refetch replaces the workspace options with the error, which
	// drops focus from a focused option to the body or the dialog.
	useEffect(() => {
		const active = document.activeElement;
		if (
			hasError &&
			(!active ||
				active === document.body ||
				active.getAttribute("role") === "dialog")
		) {
			retryButtonRef.current?.focus();
		}
	}, [hasError]);
	const errorAlert = hasError && (
		<ErrorAlert
			error={error}
			actions={
				<RetryButton
					ref={retryButtonRef}
					size="sm"
					variant="outline"
					isRetrying={isRetrying}
					onRetry={onRetry}
				/>
			}
		/>
	);
	if (workspaces === undefined) {
		return (
			errorAlert || (
				<div role="status" className="flex flex-col gap-2">
					<span className="sr-only">Loading your workspaces</span>
					<Skeleton variant="text" className="w-1/2" />
					<Skeleton variant="text" className="w-2/3" />
				</div>
			)
		);
	}
	if (workspaces.length === 0) {
		return errorAlert;
	}
	return (
		<div className="flex flex-col gap-3">
			{errorAlert}
			<div className="flex items-center gap-2">
				<p
					id={radioGroupLabelId}
					className="m-0 text-sm font-medium text-content-primary"
				>
					Workspaces used by your chats in this project
				</p>
				<Spinner loading={isRetrying} label="Refreshing your workspaces" />
			</div>
			<ul className="m-0 flex list-none flex-col gap-1 p-0 text-sm text-content-secondary">
				{workspaces.map(({ workspace, existedBeforeChat }) => (
					<li key={workspace.id} className="flex min-w-0 items-center gap-2">
						<span className="truncate">{workspace.name}</span>
						{existedBeforeChat && (
							<Badge size="sm" className="shrink-0">
								Existing workspace
							</Badge>
						)}
					</li>
				))}
			</ul>
			<RadioGroup
				aria-labelledby={radioGroupLabelId}
				value={action}
				onValueChange={(value) => {
					if (value === "leave" || value === "stop" || value === "delete") {
						onActionChange(value);
					}
				}}
			>
				<WorkspaceActionOption value="leave" label="Leave them" />
				<WorkspaceActionOption value="stop" label="Stop them" />
				<WorkspaceActionOption value="delete" label="Delete them" />
			</RadioGroup>
			{action === "delete" && (
				<DeleteConfirmationField
					confirmation={confirmation}
					// Focusing the field as it appears would end arrow-key
					// navigation in the radio group above.
					autoFocus={false}
					label={
						<>
							Type <strong>{project.name}</strong> to delete these workspaces
						</>
					}
					entity="project"
				/>
			)}
		</div>
	);
};

type WorkspaceActionOptionProps = {
	readonly value: ProjectWorkspaceAction;
	readonly label: string;
};

const WorkspaceActionOption: React.FC<WorkspaceActionOptionProps> = ({
	value,
	label,
}) => {
	const id = useId();
	return (
		<div className="flex items-center gap-2">
			<RadioGroupItem id={id} value={value} />
			<Label htmlFor={id} className="font-normal">
				{label}
			</Label>
		</div>
	);
};

type WorkspaceLinkProps = {
	readonly workspace: Workspace;
	readonly onOpen: (path: string) => void;
};

const WorkspaceLink: React.FC<WorkspaceLinkProps> = ({ workspace, onOpen }) => {
	const path = `/@${workspace.owner_name}/${workspace.name}`;
	return (
		<Link
			href={path}
			showExternalIcon={false}
			onClick={(event) => {
				// Toasts render outside the router, so plain clicks navigate here
				// and modified clicks keep the browser's behavior.
				if (
					event.button !== 0 ||
					event.metaKey ||
					event.ctrlKey ||
					event.shiftKey ||
					event.altKey
				) {
					return;
				}
				event.preventDefault();
				onOpen(path);
			}}
		>
			{workspace.name}
		</Link>
	);
};

/**
 * One toast that links every workspace that was skipped or failed, replacing
 * the toast with toastId when given.
 */
export const notifyWorkspaceActionResult = (
	action: Exclude<ProjectWorkspaceAction, "leave">,
	{ failed, skipped }: ProjectWorkspaceActionResult,
	onOpenWorkspace: (path: string) => void,
	toastId: ExternalToast["id"],
) => {
	if (failed.length === 0 && skipped.length === 0) {
		return;
	}
	const description = (
		<ul className="m-0 flex max-h-[40vh] list-none flex-col gap-1 overflow-y-auto p-0">
			{failed.map(({ workspace, error }) => (
				<li key={workspace.id}>
					<WorkspaceLink workspace={workspace} onOpen={onOpenWorkspace} />:{" "}
					{getErrorMessage(error, "Request failed.")}
				</li>
			))}
			{skipped.map((workspace) => (
				<li key={workspace.id}>
					<WorkspaceLink workspace={workspace} onOpen={onOpenWorkspace} />:
					still used by an unarchived chat.
				</li>
			))}
		</ul>
	);
	if (failed.length > 0) {
		toast.error(
			`The project was archived, but ${plural(failed.length, "workspace", "workspaces")} could not be ${action === "stop" ? "stopped" : "deleted"}.`,
			{ id: toastId, description },
		);
		return;
	}
	// Persistent like error toasts, so there is time to reach the links.
	toast.warning(
		`The project was archived. ${plural(skipped.length, "workspace", "workspaces")} still in use ${skipped.length === 1 ? "was" : "were"} skipped.`,
		{ ...persistentToast, id: toastId, description },
	);
};
