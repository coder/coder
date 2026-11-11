import { useEffect, useId, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import type { ExternalToast } from "sonner";
import { getErrorMessage, getErrorStatus } from "#/api/errors";
import {
	archiveChatProject,
	chatProjectArchiveDialogKey,
	chatProjectArchivePreview,
	chatProjectArchivePreviewKey,
} from "#/api/queries/chatProjects";
import { chatRetentionDays } from "#/api/queries/chats";
import type {
	ChatProject,
	ChatProjectArchivePreview,
	Workspace,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DeleteConfirmationField,
	useDeleteConfirmation,
} from "#/components/Dialog/DeleteDialog/DeleteConfirmationField";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { persistentToast, toast } from "#/components/Toaster/toast";
import {
	type ProjectChatWorkspace,
	type ProjectWorkspaceAction,
	type ProjectWorkspaceActionResult,
	projectChatWorkspaces,
	projectWorkspaceAction,
} from "../../../utils/agentWorkspaceUtils";
import { RetryButton } from "../../RetryButton";

const plural = (count: number, singular: string, pluralForm: string) =>
	`${count} ${count === 1 ? singular : pluralForm}`;

type ArchiveRequest = {
	readonly stopActiveChats: boolean;
	readonly workspaceAction: ProjectWorkspaceAction;
	readonly workspaces: readonly Workspace[];
};

type ArchiveProjectDialogProps = {
	/** Kept after closing so the exit animation does not flash empty. */
	readonly project: ChatProject | null;
	readonly open: boolean;
	readonly onClose: () => void;
	readonly onCloseAutoFocus?: (event: Event) => void;
};

/**
 * Confirms archiving a project: how many chats it archives, which active
 * chats it stops, how long archived chats are kept, and what happens to the
 * workspaces behind the user's own chats.
 */
export const ArchiveProjectDialog: React.FC<ArchiveProjectDialogProps> = ({
	project,
	open,
	onClose,
	onCloseAutoFocus,
}) => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const archiveMutation = useMutation(archiveChatProject(queryClient));
	// The workspace outcome replaces its archive's "Project archived" toast,
	// so one toast summarizes the whole archive.
	const workspaceMutation = useMutation({
		...projectWorkspaceAction(queryClient),
		onSuccess: (result, { action, toastId }) =>
			notifyWorkspaceActionResult(action, result, navigate, toastId),
		onError: (error, { toastId }) => {
			toast.error(
				getErrorMessage(
					error,
					"Failed to check whether your workspaces are still in use.",
				),
				{
					id: toastId,
					description:
						"The project was archived, and its workspaces were left as they were.",
				},
			);
		},
	});

	// isPending reaches the form only after the mutation cache notifies, so
	// a double click would send a second request. That mutate() call replaces
	// the first call's callbacks, so the first request's success would neither
	// close the dialog, toast, nor start the workspace action, and the second
	// request fails because the project is already archived.
	const isSubmittingRef = useRef(false);

	const archive = (
		projectToArchive: ChatProject,
		{ stopActiveChats, workspaceAction, workspaces }: ArchiveRequest,
	) => {
		if (isSubmittingRef.current) {
			return;
		}
		isSubmittingRef.current = true;
		archiveMutation.mutate(
			{ project: projectToArchive, stopActiveChats },
			{
				onSettled: () => {
					isSubmittingRef.current = false;
				},
				onSuccess: () => {
					onClose();
					const toastId = toast.success("Project archived");
					if (workspaceAction !== "leave") {
						workspaceMutation.mutate({
							action: workspaceAction,
							workspaces,
							toastId,
						});
					}
				},
				onError: async (error) => {
					// A chat can start after the preview loaded. The refreshed
					// preview shows the warning and the button that stops it,
					// and the workspace list picks up the chat's workspace.
					if (getErrorStatus(error) === 409) {
						const previewKey = chatProjectArchivePreviewKey(
							projectToArchive.id,
						);
						await queryClient.refetchQueries({
							queryKey: chatProjectArchiveDialogKey(projectToArchive.id),
						});
						const preview =
							queryClient.getQueryData<ChatProjectArchivePreview>(previewKey);
						if (preview && preview.active_chat_count > 0) {
							return;
						}
					}
					toast.error(getErrorMessage(error, "Failed to archive project."));
				},
			},
		);
	};

	return (
		<Dialog
			open={open}
			onOpenChange={(next) => {
				// The request can't be cancelled, and a 409 is shown in this dialog.
				if (!next && !archiveMutation.isPending) {
					onClose();
				}
			}}
		>
			<DialogContent onCloseAutoFocus={onCloseAutoFocus}>
				{project && (
					<ArchiveProjectForm
						project={project}
						isArchiving={archiveMutation.isPending}
						onCancel={onClose}
						onArchive={(request) => archive(project, request)}
					/>
				)}
			</DialogContent>
		</Dialog>
	);
};

type ArchiveProjectFormProps = {
	readonly project: ChatProject;
	readonly isArchiving: boolean;
	readonly onCancel: () => void;
	readonly onArchive: (request: ArchiveRequest) => void;
};

const ArchiveProjectForm: React.FC<ArchiveProjectFormProps> = ({
	project,
	isArchiving,
	onCancel,
	onArchive,
}) => {
	const queryClient = useQueryClient();
	const previewQuery = useQuery(chatProjectArchivePreview(project));
	const retentionQuery = useQuery(chatRetentionDays());
	const workspacesQuery = useQuery(projectChatWorkspaces(queryClient, project));
	const [workspaceAction, setWorkspaceAction] =
		useState<ProjectWorkspaceAction>("leave");
	const confirmation = useDeleteConfirmation(project.name, true);

	const preview = previewQuery.data;
	const activeChatCount = preview?.active_chat_count ?? 0;
	// A failed refetch keeps stale rows; show the error instead and never stop
	// or delete from them.
	const workspaces = workspacesQuery.isError
		? []
		: (workspacesQuery.data ?? []);
	const isDeleting = workspaceAction === "delete" && workspaces.length > 0;
	// Retention only describes what happens to archived chats.
	const needsRetention = preview?.chat_count !== 0;
	// A failed refetch leaves a preview that may miss newly active chats, and
	// the refetch after a 409 may reveal some. While the workspaces load or
	// refetch, the stop and delete choices may be stale; if they fail to load,
	// archiving leaves them alone.
	const canArchive =
		preview !== undefined &&
		!previewQuery.error &&
		!previewQuery.isFetching &&
		(!needsRetention || retentionQuery.data !== undefined) &&
		!workspacesQuery.isFetching &&
		!isArchiving &&
		(!isDeleting || confirmation.confirmed);

	const onSubmit = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (!canArchive) {
			return;
		}
		onArchive({
			stopActiveChats: activeChatCount > 0,
			workspaceAction: workspaces.length > 0 ? workspaceAction : "leave",
			workspaces: workspaces.map(({ workspace }) => workspace),
		});
	};

	return (
		<form className="flex min-w-0 flex-col gap-6" onSubmit={onSubmit}>
			<DialogHeader>
				<DialogTitle>Archive project</DialogTitle>
				{/* A div, because the default <p> can't hold the Skeleton's div. */}
				<DialogDescription asChild>
					<div>
						{preview === undefined && !previewQuery.error ? (
							<>
								<span className="sr-only">Loading the chat count</span>
								<Skeleton variant="text" className="w-2/3" />
							</>
						) : preview?.chat_count === 0 ? (
							<>&ldquo;{project.name}&rdquo; has no chats to archive.</>
						) : (
							<>
								Every chat in &ldquo;{project.name}&rdquo; is archived,
								including chats started by people it is shared with.
							</>
						)}
					</div>
				</DialogDescription>
			</DialogHeader>

			{/* Empty for a project with no chats and no workspaces. */}
			<div className="flex flex-col gap-4 empty:hidden">
				<ArchiveSummary
					preview={preview}
					retentionDays={retentionQuery.data?.retention_days}
					error={
						previewQuery.error ?? (needsRetention ? retentionQuery.error : null)
					}
					isRetrying={previewQuery.isFetching || retentionQuery.isFetching}
					onRetry={() => {
						void previewQuery.refetch();
						void retentionQuery.refetch();
					}}
				/>
				<ProjectWorkspacesSection
					project={project}
					workspaces={
						workspacesQuery.isError ? undefined : workspacesQuery.data
					}
					error={workspacesQuery.error}
					isRetrying={workspacesQuery.isFetching}
					onRetry={() => void workspacesQuery.refetch()}
					action={workspaceAction}
					onActionChange={setWorkspaceAction}
					confirmation={confirmation}
				/>
			</div>

			<DialogFooter>
				<Button
					type="button"
					variant="outline"
					disabled={isArchiving}
					onClick={onCancel}
				>
					Cancel
				</Button>
				<Button
					type="submit"
					variant={isDeleting ? "destructive" : "default"}
					disabled={!canArchive}
				>
					<Spinner loading={isArchiving} />
					{activeChatCount === 0
						? "Archive project"
						: activeChatCount === 1
							? "Stop chat and archive"
							: "Stop chats and archive"}
				</Button>
			</DialogFooter>
		</form>
	);
};

type ArchiveSummaryProps = {
	readonly preview: ChatProjectArchivePreview | undefined;
	readonly retentionDays: number | undefined;
	readonly error: unknown;
	readonly isRetrying: boolean;
	readonly onRetry: () => void;
};

const ArchiveSummary: React.FC<ArchiveSummaryProps> = ({
	preview,
	retentionDays,
	error,
	isRetrying,
	onRetry,
}) => {
	const errorAlert = Boolean(error) && (
		<ErrorAlert
			error={error}
			actions={
				<RetryButton
					size="sm"
					variant="outline"
					isRetrying={isRetrying}
					onRetry={onRetry}
				/>
			}
		/>
	);
	if (
		preview === undefined ||
		(preview.chat_count > 0 && retentionDays === undefined)
	) {
		return (
			errorAlert || (
				<div role="status" className="flex flex-col gap-2">
					<span className="sr-only">Loading project chats</span>
					<Skeleton variant="text" className="w-1/3" />
					<Skeleton variant="text" className="w-full" />
				</div>
			)
		);
	}
	return (
		<>
			{errorAlert}
			{preview.chat_count > 0 && retentionDays !== undefined && (
				<p className="m-0 text-sm text-content-secondary">
					{plural(preview.chat_count, "chat", "chats")} will be archived.{" "}
					{retentionDays > 0
						? `Archived chats are permanently deleted after ${plural(retentionDays, "day", "days")}. Unarchiving the project after that won't bring them back.`
						: "Archived chats are kept until you unarchive the project."}
				</p>
			)}
			{preview.active_chat_count > 0 && (
				<Alert severity="warning" prominent>
					{plural(preview.active_chat_count, "chat is", "chats are")} running.
					Archiving stops {preview.active_chat_count === 1 ? "it" : "them"} and
					discards any reply in progress. Queued messages are kept.
				</Alert>
			)}
		</>
	);
};

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

const ProjectWorkspacesSection: React.FC<ProjectWorkspacesSectionProps> = ({
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
const notifyWorkspaceActionResult = (
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
