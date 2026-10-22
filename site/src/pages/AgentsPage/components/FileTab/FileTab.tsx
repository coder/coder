import { isAxiosError } from "axios";
import { cn } from "cn";
import { TriangleAlertIcon } from "lucide-react";
import {
	lazy,
	memo,
	Suspense,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	workspaceAgentFile,
	workspaceAgentFileKey,
	workspaceAgentFileMaxTextBytes,
	writeWorkspaceAgentFile,
} from "#/api/queries/workspaceFiles";
import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { Spinner } from "#/components/Spinner/Spinner";
import { toast } from "#/components/Toaster/toast";
import { isBinaryData } from "#/modules/templates/TemplateFiles/isBinaryData";
import { getOSKey } from "#/utils/platform";
import { getPathBasename } from "../../utils/path";
import { Response } from "../ChatElements/Response";
import type { FileReveal } from "../MainTabs/useMainTabs";

const FileEditor = lazy(() => import("./FileEditor"));

/** Unsaved edits of one file. A file without a draft is clean. */
export type FileDraft = {
	content: string;
	/** The disk content the edits started from. */
	baseContent: string;
	/** Sent as `ifMatch` on save so a concurrent change on disk fails it. */
	baseEtag: string;
};

export type FileDraftUpdate = (
	current: FileDraft | undefined,
) => FileDraft | undefined;

type FileTabProps = {
	chatId: string;
	agentId: string;
	path: string;
	reveal: FileReveal | undefined;
	/**
	 * Git watcher state of the repository containing the file. A new object
	 * means the repository changed, so the file is refetched.
	 */
	repository: WorkspaceAgentRepoChanges | undefined;
	draft: FileDraft | undefined;
	onDraftChange: (path: string, update: FileDraftUpdate) => void;
	/** Closes the tab, asking first when `hasUnsavedEdits`. */
	onClose: (path: string, hasUnsavedEdits: boolean) => void;
	/** Archived chats and chats owned by another user cannot edit files. */
	readOnly: boolean;
};

const isMarkdownPath = (path: string) => /\.mdx?$/i.test(path);

// Lossy decoding would show invalid UTF-8 as replacement characters and
// write them back on save, so such files are binary. ignoreBOM keeps a BOM
// in the buffer so it round-trips.
const decodeUtf8 = (data: ArrayBuffer): string | undefined => {
	try {
		return new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(
			data,
		);
	} catch {
		return undefined;
	}
};

export const FileTab = memo<FileTabProps>(
	({
		chatId,
		agentId,
		path,
		reveal,
		repository,
		draft,
		onDraftChange,
		onClose,
		readOnly,
	}) => {
		const queryClient = useQueryClient();
		const fileQuery = useQuery({
			...workspaceAgentFile(agentId, path),
			// Files outside a repository have no change signal, so window
			// focus is the only refresh.
			refetchOnWindowFocus: repository === undefined,
		});
		const writeMutation = useMutation(
			writeWorkspaceAgentFile(queryClient, agentId),
		);
		const [hasSaveConflict, setHasSaveConflict] = useState(false);
		const isMarkdown = isMarkdownPath(path);
		const [mode, setMode] = useState<"edit" | "preview">(
			isMarkdown && reveal?.line === undefined ? "preview" : "edit",
		);
		const rootRef = useRef<HTMLDivElement>(null);
		// Cleared once applied, so an editor that remounts, such as after
		// switching to Preview and back, does not apply it again.
		const [pendingReveal, setPendingReveal] = useState(reveal);
		const [prevReveal, setPrevReveal] = useState(reveal);
		if (reveal !== prevReveal) {
			setPrevReveal(reveal);
			setPendingReveal(reveal);
			if (reveal?.line !== undefined) {
				setMode("edit");
			}
		}
		// Banner and Save buttons disable or unmount while their action runs,
		// which drops focus to the page; it returns to the editor once the
		// action settles, unless the user moved it elsewhere meanwhile.
		const restoreFocus = () => {
			const focused = document.activeElement;
			if (
				!focused ||
				focused === document.body ||
				rootRef.current?.contains(focused)
			) {
				setPendingReveal({ focus: true });
			}
		};

		const refetchFile = useEffectEvent(() => {
			void fileQuery.refetch();
		});
		// The mount-time read already covers the repository state at mount.
		// Later states restart a read in flight so the newest content wins.
		const refetchedRepositoryRef = useRef(repository);
		useEffect(() => {
			if (refetchedRepositoryRef.current === repository) {
				return;
			}
			refetchedRepositoryRef.current = repository;
			if (repository) {
				refetchFile();
			}
		}, [repository]);

		const data = fileQuery.data;
		const isImage = data?.contentType.startsWith("image/") ?? false;
		const text =
			data && !isImage && !data.partial ? decodeUtf8(data.data) : undefined;
		const isText = text !== undefined && !isBinaryData(text);
		const value = draft?.content ?? text ?? "";
		// A failed refetch keeps the content of the last successful read, so
		// the buffer outlives the file's deletion.
		const isDeleted =
			data !== undefined &&
			isAxiosError(fileQuery.error) &&
			fileQuery.error.response?.status === 404;
		const hasExternalChange =
			draft !== undefined &&
			data !== undefined &&
			data.etag !== draft.baseEtag &&
			!writeMutation.isPending;

		const handleChange = (content: string) => {
			if (!data || text === undefined) {
				return;
			}
			// During a save the disk content is unknown, so a buffer that
			// returns to the old base stays a draft until the save settles.
			const isSaving = writeMutation.isPending;
			onDraftChange(path, (current) => {
				const base = current ?? { baseContent: text, baseEtag: data.etag };
				return content === base.baseContent && !isSaving
					? undefined
					: { ...base, content };
			});
		};

		const write = (content: string, ifMatch: string | undefined) => {
			if (readOnly || writeMutation.isPending) {
				return;
			}
			// A read that started before the write would land after it and
			// replace the saved content with the old one.
			void queryClient.cancelQueries({
				queryKey: workspaceAgentFileKey(agentId, path),
			});
			writeMutation.mutate(
				{ path, content, ifMatch },
				{
					onSuccess: ({ etag }) => {
						restoreFocus();
						setHasSaveConflict(false);
						// The saved snapshot is the new base, so edits made during
						// the save, including an undo, stay dirty against it.
						onDraftChange(path, (current) =>
							!current || current.content === content
								? undefined
								: { ...current, baseContent: content, baseEtag: etag },
						);
					},
					onError: (error) => {
						onDraftChange(path, (current) =>
							!current || current.content === current.baseContent
								? undefined
								: current,
						);
						if (isAxiosError(error) && error.response?.status === 412) {
							setHasSaveConflict(true);
							// The agent also rejects a file deleted since the read;
							// a read tells a deletion from a change.
							void fileQuery.refetch();
							return;
						}
						toast.error(getErrorMessage(error, "Failed to save file."), {
							description: getErrorDetail(error),
						});
					},
				},
			);
		};
		const save = (ifMatch: string | undefined) => {
			if (draft) {
				write(draft.content, ifMatch);
			}
		};
		// A deleted file has no version to conflict with.
		const handleSave = () =>
			save(isDeleted ? undefined : draft?.baseEtag || undefined);

		const handleReload = () => {
			const reloadedDraft = draft;
			void fileQuery.refetch().then((result) => {
				if (!result.isSuccess) {
					toast.error(getErrorMessage(result.error, "Failed to reload file."));
					return;
				}
				setHasSaveConflict(false);
				restoreFocus();
				// Edits typed while the read was in flight stay a draft.
				onDraftChange(path, (current) =>
					current === reloadedDraft ? undefined : current,
				);
			});
		};

		const handleKeepMine = () => {
			if (!data || text === undefined) {
				return;
			}
			restoreFocus();
			onDraftChange(
				path,
				(current) =>
					current &&
					(current.content === text
						? undefined
						: { ...current, baseContent: text, baseEtag: data.etag }),
			);
		};

		let body: React.ReactNode;
		let showsEditor = false;
		if (fileQuery.isLoading) {
			body = <Loader label="Loading file" />;
		} else if (!data) {
			body = (
				<FileLoadError
					error={fileQuery.error}
					onRetry={() => void fileQuery.refetch()}
				/>
			);
		} else if (data.partial === "too-large") {
			body = (
				<EmptyState
					message="File too large"
					description={`Files over ${workspaceAgentFileMaxTextBytes / (1 << 20)} MiB do not open here. Open it in an editor in the workspace.`}
				/>
			);
		} else if (data.partial === "binary") {
			body = (
				<EmptyState
					message="Binary file"
					description="Binary files cannot be displayed."
				/>
			);
		} else if (isImage) {
			body = (
				<ImagePreview
					data={data.data}
					contentType={data.contentType}
					alt={getPathBasename(path)}
				/>
			);
		} else if (!isText) {
			body = (
				<EmptyState
					message="Binary file"
					description={`${data.data.byteLength.toLocaleString("en-US")} bytes. Binary files cannot be displayed.`}
				/>
			);
		} else if (isMarkdown && mode === "preview" && !value) {
			body = <EmptyState message="Empty file" />;
		} else if (isMarkdown && mode === "preview") {
			body = (
				<div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
					<Response>{value}</Response>
				</div>
			);
		} else {
			showsEditor = true;
			body = (
				<Suspense fallback={<Loader label="Loading editor" />}>
					<FileEditor
						chatId={chatId}
						path={path}
						value={value}
						reveal={pendingReveal}
						readOnly={readOnly}
						onChange={handleChange}
						onSave={handleSave}
						onRevealed={() => setPendingReveal(undefined)}
					/>
				</Suspense>
			);
		}

		const isLoading = fileQuery.isLoading;
		// Content without an editor takes focus on the tab itself.
		useEffect(() => {
			if (!pendingReveal || isLoading || showsEditor) {
				return;
			}
			if (pendingReveal.focus) {
				rootRef.current?.focus();
			}
			setPendingReveal(undefined);
		}, [pendingReveal, isLoading, showsEditor]);

		return (
			<div
				ref={rootRef}
				tabIndex={-1}
				className="flex min-h-0 flex-1 flex-col outline-none"
			>
				{/* A fixed height, with notices inline, so the file content does not
				    move when a notice or a spinner appears. */}
				<div className="flex h-8 shrink-0 items-center gap-2 border-0 border-b border-solid border-border px-3">
					<span
						className="min-w-0 flex-1 truncate font-mono text-xs text-content-secondary"
						title={path}
					>
						{path}
					</span>
					{isDeleted ? (
						<FileNotice
							title="Deleted on disk"
							description="The file was deleted on disk. Recreate it from the open content, or close the tab."
						>
							{!readOnly && (
								<PendingButton
									isPending={writeMutation.isPending}
									disabled={writeMutation.isPending}
									onClick={() => write(value, undefined)}
								>
									Recreate
								</PendingButton>
							)}
							<Button
								size="xs"
								variant="outline"
								onClick={() => onClose(path, draft !== undefined)}
							>
								Close
							</Button>
						</FileNotice>
					) : hasSaveConflict && draft ? (
						<FileNotice
							title="Changed on disk"
							description="The file changed on disk after you started editing."
						>
							<PendingButton
								isPending={fileQuery.isFetching}
								disabled={fileQuery.isFetching || writeMutation.isPending}
								onClick={handleReload}
							>
								Reload
							</PendingButton>
							<PendingButton
								isPending={writeMutation.isPending}
								disabled={fileQuery.isFetching || writeMutation.isPending}
								onClick={() => save(undefined)}
							>
								Overwrite
							</PendingButton>
						</FileNotice>
					) : (
						hasExternalChange && (
							<FileNotice
								title="Changed on disk"
								description="The file changed on disk while you have unsaved edits."
							>
								<PendingButton
									isPending={fileQuery.isFetching}
									disabled={fileQuery.isFetching}
									onClick={handleReload}
								>
									Reload
								</PendingButton>
								<Button size="xs" variant="outline" onClick={handleKeepMine}>
									Keep mine
								</Button>
							</FileNotice>
						)
					)}
					{isMarkdown && isText && (
						<div role="group" aria-label="View mode" className="flex gap-1">
							<Button
								size="xs"
								variant={mode === "preview" ? "outline" : "subtle"}
								aria-pressed={mode === "preview"}
								onClick={() => setMode("preview")}
							>
								Preview
							</Button>
							<Button
								size="xs"
								variant={mode === "edit" ? "outline" : "subtle"}
								aria-pressed={mode === "edit"}
								onClick={() => setMode("edit")}
							>
								Edit
							</Button>
						</div>
					)}
					{isText && !readOnly && (
						<PendingButton
							isPending={writeMutation.isPending}
							title={`Save (${getOSKey()}+S)`}
							disabled={!draft || writeMutation.isPending}
							onClick={handleSave}
						>
							Save
						</PendingButton>
					)}
				</div>
				<div className="relative flex min-h-0 flex-1 flex-col">{body}</div>
			</div>
		);
	},
);

type PendingButtonProps = React.ComponentProps<typeof Button> & {
	isPending: boolean;
};

/** Keeps its size while pending: the spinner covers the label. */
const PendingButton: React.FC<PendingButtonProps> = ({
	isPending,
	children,
	...props
}) => (
	<Button size="xs" variant="outline" className="relative" {...props}>
		<span className={cn(isPending && "opacity-0")}>{children}</span>
		{isPending && (
			<Spinner aria-hidden size="sm" loading className="absolute" />
		)}
	</Button>
);

type FileNoticeProps = {
	title: string;
	description: string;
	/** The notice's actions. */
	children: React.ReactNode;
};

/** A warning in the file header, where it does not move the content. */
const FileNotice: React.FC<FileNoticeProps> = ({
	title,
	description,
	children,
}) => (
	<div
		role="alert"
		title={description}
		className="flex min-w-0 items-center gap-1.5"
	>
		<TriangleAlertIcon
			aria-hidden
			className="size-icon-sm shrink-0 text-content-warning"
		/>
		<span className="truncate text-xs font-medium text-content-warning">
			{title}
			<span className="sr-only">. {description}</span>
		</span>
		{children}
	</div>
);

type FileLoadErrorProps = {
	error: unknown;
	onRetry: () => void;
};

const FileLoadError: React.FC<FileLoadErrorProps> = ({ error, onRetry }) => {
	const status = isAxiosError(error) ? error.response?.status : undefined;
	if (status === 404) {
		return (
			<EmptyState
				message="File not found"
				description="The file may have been moved or deleted."
			/>
		);
	}
	if (status === 403) {
		return (
			<EmptyState
				message="Permission denied"
				description="You do not have permission to read this file."
			/>
		);
	}
	return (
		<EmptyState
			message="Could not load file"
			description={getErrorMessage(
				error,
				"The workspace agent could not read this file.",
			)}
			cta={
				<Button size="sm" variant="outline" onClick={onRetry}>
					Retry
				</Button>
			}
		/>
	);
};

type ImagePreviewProps = {
	data: ArrayBuffer;
	contentType: string;
	alt: string;
};

const ImagePreview: React.FC<ImagePreviewProps> = ({
	data,
	contentType,
	alt,
}) => {
	const [url, setUrl] = useState<string>();

	useEffect(() => {
		const objectUrl = URL.createObjectURL(
			new Blob([data], { type: contentType }),
		);
		setUrl(objectUrl);
		return () => URL.revokeObjectURL(objectUrl);
	}, [data, contentType]);

	if (!url) {
		return null;
	}
	return (
		<div className="flex min-h-0 flex-1 items-center justify-center overflow-auto p-4">
			<img
				src={url}
				alt={alt}
				className="max-h-full max-w-full object-contain"
			/>
		</div>
	);
};
