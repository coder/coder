import { useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
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
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { toast } from "#/components/Toaster/toast";
import { RetryButton } from "../../RetryButton";

const plural = (count: number, singular: string, pluralForm: string) =>
	`${count} ${count === 1 ? singular : pluralForm}`;

type ArchiveRequest = {
	readonly stopActiveChats: boolean;
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
 * chats it stops, and how long archived chats are kept.
 */
export const ArchiveProjectDialog: React.FC<ArchiveProjectDialogProps> = ({
	project,
	open,
	onClose,
	onCloseAutoFocus,
}) => {
	const queryClient = useQueryClient();
	const archiveMutation = useMutation(archiveChatProject(queryClient));

	// isPending reaches the form only after the mutation cache notifies, so
	// a double click would send a second request. That mutate() call replaces
	// the first call's callbacks, so the first request's success would neither
	// close the dialog nor toast, and the second request fails because the
	// project is already archived.
	const isSubmittingRef = useRef(false);

	const archive = (
		projectToArchive: ChatProject,
		{ stopActiveChats }: ArchiveRequest,
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
					toast.success("Project archived");
				},
				onError: async (error) => {
					// A chat can start after the preview loaded. The refreshed
					// preview shows the warning and the button that stops it.
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
	const previewQuery = useQuery(chatProjectArchivePreview(project));
	const retentionQuery = useQuery(chatRetentionDays());

	const preview = previewQuery.data;
	const activeChatCount = preview?.active_chat_count ?? 0;
	// Retention only describes what happens to archived chats.
	const needsRetention = preview?.chat_count !== 0;
	// A failed refetch leaves a preview that may miss newly active chats, and
	// the refetch after a 409 may reveal some.
	const canArchive =
		preview !== undefined &&
		!previewQuery.error &&
		!previewQuery.isFetching &&
		(!needsRetention || retentionQuery.data !== undefined) &&
		!isArchiving;

	const onSubmit = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (!canArchive) {
			return;
		}
		onArchive({ stopActiveChats: activeChatCount > 0 });
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
								Archiving it archives every chat in &ldquo;{project.name}
								&rdquo;, including chats started by people it is shared with.
							</>
						)}
					</div>
				</DialogDescription>
			</DialogHeader>

			{/* Empty for a project with no chats. */}
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
				<Button type="submit" disabled={!canArchive}>
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
