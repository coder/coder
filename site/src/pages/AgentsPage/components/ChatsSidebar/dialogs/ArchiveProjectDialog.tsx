import { useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { getErrorMessage, getErrorStatus } from "#/api/errors";
import {
	archiveChatProject,
	chatProjectArchivePreview,
	chatProjectArchivePreviewKey,
} from "#/api/queries/chatProjects";
import { chatRetentionDays } from "#/api/queries/chats";
import type {
	ChatProject,
	ChatProjectArchivePreview,
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
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { toast } from "#/components/Toaster/toast";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	type ProjectChatWorkspace,
	type ProjectWorkspaceAction,
	projectChatWorkspaces,
	runProjectWorkspaceAction,
} from "../../../utils/projectArchiveWorkspaces";

const plural = (count: number, singular: string, pluralForm: string) =>
	`${count} ${count === 1 ? singular : pluralForm}`;

const retentionSentence = (retentionDays: number) =>
	retentionDays > 0
		? `Archived chats are permanently deleted after ${plural(retentionDays, "day", "days")}. Unarchiving the project after that won't bring them back.`
		: "Archived chats are kept until you unarchive the project.";

type ArchiveRequest = {
	readonly stopActiveChats: boolean;
	readonly workspaceAction: ProjectWorkspaceAction;
	readonly workspaces: readonly ProjectChatWorkspace[];
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

	const archive = (
		archivedProject: ChatProject,
		{ stopActiveChats, workspaceAction, workspaces }: ArchiveRequest,
	) => {
		archiveMutation.mutate(
			{ project: archivedProject, stopActiveChats },
			{
				onSuccess: () => {
					onClose();
					toast.success("Project archived");
					if (workspaceAction !== "leave") {
						void runProjectWorkspaceAction(
							queryClient,
							workspaceAction,
							workspaces.map(({ workspace }) => workspace),
							navigate,
						);
					}
				},
				onError: async (error) => {
					// A chat can start after the preview loaded. The refreshed
					// preview shows the warning and the button that stops it.
					if (getErrorStatus(error) === 409) {
						const previewKey = chatProjectArchivePreviewKey(archivedProject.id);
						await queryClient.refetchQueries({ queryKey: previewKey });
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
				// Closing mid-request would drop the workspace follow-up.
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
	const { user } = useAuthenticated();
	const previewQuery = useQuery(chatProjectArchivePreview(project));
	const retentionQuery = useQuery(chatRetentionDays());
	const workspacesQuery = useQuery(
		projectChatWorkspaces(queryClient, project, user.id),
	);
	const [workspaceAction, setWorkspaceAction] =
		useState<ProjectWorkspaceAction>("leave");
	const confirmation = useDeleteConfirmation(project.name, true);
	const radioGroupLabelId = useId();

	const preview = previewQuery.data;
	const retentionDays = retentionQuery.data?.retention_days;
	const activeChatCount = preview?.active_chat_count ?? 0;
	const workspaces = workspacesQuery.data ?? [];
	const isDeleting = workspaceAction === "delete" && workspaces.length > 0;
	const summaryError = previewQuery.error ?? retentionQuery.error;
	const canArchive =
		preview !== undefined &&
		retentionDays !== undefined &&
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
			workspaces,
		});
	};

	return (
		<form className="flex flex-col gap-6" onSubmit={onSubmit}>
			<DialogHeader>
				<DialogTitle>Archive project</DialogTitle>
				<DialogDescription>
					Every chat in &ldquo;{project.name}&rdquo; is archived, including
					chats started by people it is shared with.
				</DialogDescription>
			</DialogHeader>

			<div className="flex flex-col gap-4">
				{summaryError ? (
					<ErrorAlert
						error={summaryError}
						actions={
							<Button
								type="button"
								size="sm"
								variant="outline"
								onClick={() => {
									void previewQuery.refetch();
									void retentionQuery.refetch();
								}}
							>
								Retry
							</Button>
						}
					/>
				) : preview === undefined || retentionDays === undefined ? (
					<div role="status" className="flex flex-col gap-2">
						<span className="sr-only">Loading project chats</span>
						<Skeleton variant="text" className="w-1/3" />
						<Skeleton variant="text" className="w-full" />
					</div>
				) : (
					<>
						<p className="m-0 text-sm text-content-secondary">
							{plural(preview.chat_count, "chat", "chats")} will be archived.{" "}
							{retentionSentence(retentionDays)}
						</p>
						{activeChatCount > 0 && (
							<Alert severity="warning" prominent>
								{activeChatCount === 1
									? "1 chat is running."
									: `${activeChatCount} chats are running.`}{" "}
								Archiving stops them and discards any reply in progress. Queued
								messages are kept.
							</Alert>
						)}
					</>
				)}

				{workspacesQuery.isLoading ? (
					<div role="status" className="flex flex-col gap-2">
						<span className="sr-only">Loading your workspaces</span>
						<Skeleton variant="text" className="w-1/2" />
						<Skeleton variant="text" className="w-2/3" />
					</div>
				) : workspacesQuery.error ? (
					<ErrorAlert
						error={workspacesQuery.error}
						actions={
							<Button
								type="button"
								size="sm"
								variant="outline"
								onClick={() => void workspacesQuery.refetch()}
							>
								Retry
							</Button>
						}
					/>
				) : (
					workspaces.length > 0 && (
						<div className="flex flex-col gap-3">
							<p
								id={radioGroupLabelId}
								className="m-0 text-sm font-medium text-content-primary"
							>
								Workspaces used by your chats in this project
							</p>
							<ul className="m-0 flex list-none flex-col gap-1 p-0 text-sm text-content-secondary">
								{workspaces.map(({ workspace, existedBeforeChat }) => (
									<li key={workspace.id} className="flex items-center gap-2">
										<span className="truncate">{workspace.name}</span>
										{existedBeforeChat && (
											<Badge size="sm">existing workspace</Badge>
										)}
									</li>
								))}
							</ul>
							<RadioGroup
								aria-labelledby={radioGroupLabelId}
								value={workspaceAction}
								onValueChange={(value) => {
									if (
										value === "leave" ||
										value === "stop" ||
										value === "delete"
									) {
										setWorkspaceAction(value);
									}
								}}
							>
								<WorkspaceActionOption value="leave" label="Leave them" />
								<WorkspaceActionOption value="stop" label="Stop them" />
								<WorkspaceActionOption value="delete" label="Delete them" />
							</RadioGroup>
							{isDeleting && (
								<DeleteConfirmationField
									confirmation={confirmation}
									label={
										<>
											Type <strong>{project.name}</strong> to delete these
											workspaces
										</>
									}
									entity="project"
								/>
							)}
						</div>
					)
				)}
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
					{activeChatCount > 0 ? "Stop chats and archive" : "Archive project"}
				</Button>
			</DialogFooter>
		</form>
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
