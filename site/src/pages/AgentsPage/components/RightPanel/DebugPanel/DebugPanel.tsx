import { saveAs } from "file-saver";
import { DownloadIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { type QueryClient, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { chatDebugRuns } from "#/api/queries/chats";
import type { ChatDebugRunSummary } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Spinner } from "#/components/Spinner/Spinner";
import { DebugRunList } from "./DebugRunList";
import { buildChatExportBlob, debugExportFilename } from "./debugPanelUtils";

interface DebugPanelProps {
	chatId: string;
	isVisible?: boolean;
	/** Override for the download function; used in tests. */
	download?: (blob: Blob, filename: string) => void;
}

export const DebugPanel: FC<DebugPanelProps> = ({
	chatId,
	isVisible = false,
	download = saveAs,
}) => {
	const runsQuery = useQuery({
		...chatDebugRuns(chatId),
		enabled: isVisible,
	});

	const queryClient = useQueryClient();
	const [isExporting, setIsExporting] = useState(false);

	const sortedRuns = (runsQuery.data ?? []).toSorted((left, right) => {
		const rightTime = Date.parse(right.started_at || right.updated_at) || 0;
		const leftTime = Date.parse(left.started_at || left.updated_at) || 0;
		return rightTime - leftTime;
	});

	const hasRunsData = runsQuery.data !== undefined;
	const refreshWarning =
		runsQuery.isError && hasRunsData ? (
			<div className="p-4 pb-0">
				<Alert severity="warning">
					<p className="text-sm text-content-primary">
						{getErrorMessage(
							runsQuery.error,
							"Unable to refresh debug runs. Showing cached data.",
						)}
					</p>
				</Alert>
			</div>
		) : null;

	let content: ReactNode;
	if (runsQuery.isError && !hasRunsData) {
		content = (
			<div className="p-4">
				<Alert severity="error" prominent>
					<p className="text-sm text-content-primary">
						{getErrorMessage(
							runsQuery.error,
							"Unable to load debug panel data.",
						)}
					</p>
				</Alert>
			</div>
		);
	} else if (runsQuery.isLoading) {
		content = (
			<div className="flex items-center gap-2 p-4 text-sm text-content-secondary">
				<Spinner size="sm" loading />
				Loading debug runs...
			</div>
		);
	} else if (sortedRuns.length === 0) {
		content = (
			<>
				{refreshWarning}
				<div className="flex flex-col gap-2 p-4 text-sm text-content-secondary">
					<p className="font-medium text-content-primary">
						No debug runs recorded yet
					</p>
					<p>
						Debug logging captures LLM request/response data for each chat turn,
						title generation, and compaction operation.
					</p>
					<p>Send a message in this chat to start capturing debug data.</p>
				</div>
			</>
		);
	} else {
		content = (
			<>
				{refreshWarning}
				<ExportAllButton
					chatId={chatId}
					runs={sortedRuns}
					queryClient={queryClient}
					isExporting={isExporting}
					setIsExporting={setIsExporting}
					download={download}
				/>
				<DebugRunList
					runs={sortedRuns}
					chatId={chatId}
					isVisible={isVisible}
					download={download}
				/>
			</>
		);
	}

	return (
		<ScrollArea
			className="h-full"
			viewportClassName="h-full [&>div]:!block [&>div]:!w-full"
			scrollBarClassName="w-1.5"
		>
			<div className="min-h-full w-full min-w-0 overflow-x-hidden">
				{content}
			</div>
		</ScrollArea>
	);
};

// ---------------------------------------------------------------------------
// ExportAllButton: fetches full detail for every run and downloads
// the combined payload as a single JSON file.
// ---------------------------------------------------------------------------

interface ExportAllButtonProps {
	chatId: string;
	runs: readonly ChatDebugRunSummary[];
	queryClient: QueryClient;
	isExporting: boolean;
	setIsExporting: (v: boolean) => void;
	download: (blob: Blob, filename: string) => void;
}

const ExportAllButton: FC<ExportAllButtonProps> = ({
	chatId,
	runs,
	queryClient,
	isExporting,
	setIsExporting,
	download,
}) => {
	const handleExportAll = async () => {
		setIsExporting(true);
		try {
			// Fetch full detail for every run in parallel, falling
			// back to cached data when the detail query was already
			// fetched (e.g. the user expanded a run card earlier).
			const details = await Promise.all(
				runs.map((run) =>
					queryClient
						.fetchQuery({
							queryKey: ["chats", chatId, "debug-runs", run.id],
							queryFn: () => API.experimental.getChatDebugRun(chatId, run.id),
							staleTime: 30_000,
						})
						.catch(() => run as unknown as Record<string, unknown>),
				),
			);

			const blob = buildChatExportBlob(
				chatId,
				details as unknown as Record<string, unknown>[],
			);
			download(blob, debugExportFilename(chatId));
		} catch (error) {
			console.error(error);
			toast.error("Failed to export debug logs.", {
				description: getErrorDetail(error),
			});
		}
		setIsExporting(false);
	};

	return (
		<div className="flex justify-end px-4 pt-4">
			<Button
				variant="outline"
				size="sm"
				disabled={isExporting || runs.length === 0}
				onClick={handleExportAll}
				aria-label="Export all debug runs"
			>
				{isExporting ? (
					<Spinner size="sm" loading />
				) : (
					<DownloadIcon className="size-4" />
				)}
				Export all
			</Button>
		</div>
	);
};
