import { isAxiosError } from "axios";
import {
	lazy,
	memo,
	Suspense,
	useEffect,
	useEffectEvent,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	workspaceAgentFile,
	workspaceAgentFileKey,
	writeWorkspaceAgentFile,
} from "#/api/queries/workspaceFiles";
import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
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
};

const isMarkdownPath = (path: string) => /\.mdx?$/i.test(path);

export const FileTab = memo<FileTabProps>(
	({ agentId, path, reveal, repository, draft, onDraftChange }) => {
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
			isMarkdown && !reveal ? "preview" : "edit",
		);
		const [prevReveal, setPrevReveal] = useState(reveal);
		if (reveal !== prevReveal) {
			setPrevReveal(reveal);
			if (reveal) {
				setMode("edit");
			}
		}

		const refetchFile = useEffectEvent(() => {
			// cancelRefetch: false makes the mount-time call join the initial
			// fetch instead of restarting it.
			void fileQuery.refetch({ cancelRefetch: false });
		});
		useEffect(() => {
			if (repository) {
				refetchFile();
			}
		}, [repository]);

		const data = fileQuery.data;
		const isImage = data?.contentType.startsWith("image/") ?? false;
		const text =
			data && !isImage ? new TextDecoder().decode(data.data) : undefined;
		const isText = text !== undefined && !isBinaryData(text);
		const value = draft?.content ?? text ?? "";
		const hasExternalChange =
			draft !== undefined &&
			data !== undefined &&
			data.etag !== draft.baseEtag &&
			!writeMutation.isPending;

		const handleChange = (content: string) => {
			if (!data || text === undefined) {
				return;
			}
			onDraftChange(path, (current) => {
				const base = current ?? { baseContent: text, baseEtag: data.etag };
				return content === base.baseContent ? undefined : { ...base, content };
			});
		};

		const save = (ifMatch: string | undefined) => {
			if (!draft || writeMutation.isPending) {
				return;
			}
			const saved = draft;
			// A read that started before the write would land after it and
			// replace the saved content with the old one.
			void queryClient.cancelQueries({
				queryKey: workspaceAgentFileKey(agentId, path),
			});
			writeMutation.mutate(
				{ path, content: saved.content, ifMatch },
				{
					onSuccess: ({ etag }) => {
						setHasSaveConflict(false);
						onDraftChange(path, (current) =>
							!current || current.content === saved.content
								? undefined
								: { ...current, baseContent: saved.content, baseEtag: etag },
						);
					},
					onError: (error) => {
						if (isAxiosError(error) && error.response?.status === 412) {
							setHasSaveConflict(true);
							return;
						}
						toast.error(getErrorMessage(error, "Failed to save file."), {
							description: getErrorDetail(error),
						});
					},
				},
			);
		};
		const handleSave = () => save(draft?.baseEtag || undefined);

		const handleReload = () => {
			setHasSaveConflict(false);
			onDraftChange(path, () => undefined);
			void fileQuery.refetch();
		};

		const handleKeepMine = () => {
			if (!data || text === undefined) {
				return;
			}
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
		if (fileQuery.isLoading) {
			body = <Loader label="Loading file" />;
		} else if (!data) {
			body = (
				<FileLoadError
					error={fileQuery.error}
					onRetry={() => void fileQuery.refetch()}
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
			body = (
				<Suspense fallback={<Loader label="Loading editor" />}>
					<FileEditor
						path={path}
						value={value}
						reveal={reveal}
						onChange={handleChange}
						onSave={handleSave}
					/>
				</Suspense>
			);
		}

		return (
			<div className="flex min-h-0 flex-1 flex-col">
				<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border px-3 py-1">
					<span
						className="min-w-0 flex-1 truncate font-mono text-xs text-content-secondary"
						title={path}
					>
						{path}
					</span>
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
					{isText && (
						<Button
							size="xs"
							variant="outline"
							title={`Save (${getOSKey()}+S)`}
							disabled={!draft || writeMutation.isPending}
							onClick={handleSave}
						>
							<Spinner size="sm" loading={writeMutation.isPending} />
							Save
						</Button>
					)}
				</div>
				{hasSaveConflict && (
					<ChangedOnDiskAlert
						description="The file changed on disk after you started editing."
						keepLabel="Overwrite"
						isPending={writeMutation.isPending}
						onReload={handleReload}
						onKeep={() => save(undefined)}
					/>
				)}
				{!hasSaveConflict && hasExternalChange && (
					<ChangedOnDiskAlert
						description="The file changed on disk while you have unsaved edits."
						keepLabel="Keep mine"
						isPending={false}
						onReload={handleReload}
						onKeep={handleKeepMine}
					/>
				)}
				<div className="relative flex min-h-0 flex-1 flex-col">{body}</div>
			</div>
		);
	},
);

type ChangedOnDiskAlertProps = {
	description: string;
	keepLabel: string;
	isPending: boolean;
	onReload: () => void;
	onKeep: () => void;
};

const ChangedOnDiskAlert: React.FC<ChangedOnDiskAlertProps> = ({
	description,
	keepLabel,
	isPending,
	onReload,
	onKeep,
}) => (
	<Alert
		severity="warning"
		prominent
		className="rounded-none border-x-0 border-t-0 px-3 py-2"
		actions={
			<>
				<Button size="xs" variant="outline" onClick={onReload}>
					Reload
				</Button>
				<Button
					size="xs"
					variant="outline"
					disabled={isPending}
					onClick={onKeep}
				>
					<Spinner size="sm" loading={isPending} />
					{keepLabel}
				</Button>
			</>
		}
	>
		<AlertTitle>Changed on disk</AlertTitle>
		<p className="m-0 text-xs text-content-secondary">{description}</p>
	</Alert>
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
