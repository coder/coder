import { useId, useRef, useState } from "react";
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
import { toast } from "#/components/Toaster/toast";
import {
	type ProjectChatWorkspace,
	type ProjectWorkspaceAction,
	type ProjectWorkspaceActionResult,
	projectChatWorkspaces,
	projectWorkspaceAction,
} from "../../../utils/agentWorkspaceUtils";

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
	// The workspace outcome replaces the "Project archived" toast, so one
	// toast summarizes the whole archive.
	const archivedToastIdRef = useRef<ExternalToast["id"]>(undefined);
	const workspaceMutation = useMutation({
		...projectWorkspaceAction(queryClient),
		onSuccess: (result, { action }) =>
			notifyWorkspaceActionResult(
				action,
				result,
				navigate,
				archivedToastIdRef.current,
			),
		onError: (error) => {
			toast.error(
				getErrorMessage(
					error,
					"Failed to check whether your workspaces are still in use.",
				),
				{
					id: archivedToastIdRef.current,
					description:
						"The project was archived, and its workspaces were left as they were.",
				},
			);
		},
	});

	// isPending reaches the form only after the mutation cache notifies, so
	// a double click would send a second request. Its 409 would replace the
	// first request's result, skipping the toast and the workspace action.
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
					archivedToastIdRef.current = toast.success("Project archived");
					if (workspaceAction !== "leave") {
						workspaceMutation.mutate({ action: workspaceAction, workspaces });
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
	const workspaces = workspacesQuery.data ?? [];
	const isDeleting = workspaceAction === "delete" && workspaces.length > 0;
	// A failed refetch leaves a preview that may miss newly active chats.
	// Until the workspaces load, the user hasn't seen the stop and delete
	// choices; if they fail to load, archiving leaves them alone.
	const canArchive =
		preview !== undefined &&
		!previewQuery.error &&
		retentionQuery.data !== undefined &&
		!workspacesQuery.isPending &&
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
					error={previewQuery.error ?? retentionQuery.error}
					onRetry={() => {
						void previewQuery.refetch();
						void retentionQuery.refetch();
					}}
				/>
				<ProjectWorkspacesSection
					project={project}
					workspaces={workspacesQuery.data}
					error={workspacesQuery.error}
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

type RetryButtonProps = {
	readonly onRetry: () => void;
};

const RetryButton: React.FC<RetryButtonProps> = ({ onRetry }) => (
	<Button type="button" size="sm" variant="outline" onClick={onRetry}>
		Retry
	</Button>
);

type ArchiveSummaryProps = {
	readonly preview: ChatProjectArchivePreview | undefined;
	readonly retentionDays: number | undefined;
	readonly error: unknown;
	readonly onRetry: () => void;
};

const ArchiveSummary: React.FC<ArchiveSummaryProps> = ({
	preview,
	retentionDays,
	error,
	onRetry,
}) => {
	const errorAlert = Boolean(error) && (
		<ErrorAlert error={error} actions={<RetryButton onRetry={onRetry} />} />
	);
	if (preview === undefined || retentionDays === undefined) {
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
			{preview.chat_count > 0 && (
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
	readonly onRetry: () => void;
	readonly action: ProjectWorkspaceAction;
	readonly onActionChange: (action: ProjectWorkspaceAction) => void;
	readonly confirmation: ReturnType<typeof useDeleteConfirmation>;
};

/** Offers to stop or delete the workspaces behind the user's own chats. */
const ProjectWorkspacesSection: React.FC<ProjectWorkspacesSectionProps> = ({
	project,
	workspaces,
	error,
	onRetry,
	action,
	onActionChange,
	confirmation,
}) => {
	const radioGroupLabelId = useId();
	const errorAlert = Boolean(error) && (
		<ErrorAlert error={error} actions={<RetryButton onRetry={onRetry} />} />
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
			<p
				id={radioGroupLabelId}
				className="m-0 text-sm font-medium text-content-primary"
			>
				Workspaces used by your chats in this project
			</p>
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
		<ul className="m-0 flex list-none flex-col gap-1 p-0">
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
		{ id: toastId, description, duration: Number.POSITIVE_INFINITY },
	);
};
