import { saveAs } from "file-saver";
import { DownloadIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { chatDebugRun, chatDebugRuns } from "#/api/queries/chats";
import type { ChatDebugRunSummary } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Spinner } from "#/components/Spinner/Spinner";
import { DebugRunList } from "./DebugRunList";
import {
	buildChatDebugExport,
	buildDebugExportBlob,
	type DownloadDebugFile,
	debugExportFilename,
} from "./debugExport";

interface DebugPanelProps {
	chatId: string;
	isVisible?: boolean;
	download?: DownloadDebugFile;
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
				<ExportAllDebugRunsButton
					chatId={chatId}
					runs={sortedRuns}
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

interface ExportAllDebugRunsButtonProps {
	chatId: string;
	runs: readonly ChatDebugRunSummary[];
	download: DownloadDebugFile;
}

const ExportAllDebugRunsButton: FC<ExportAllDebugRunsButtonProps> = ({
	chatId,
	runs,
	download,
}) => {
	const queryClient = useQueryClient();
	const [isExporting, setIsExporting] = useState(false);

	return (
		<div className="flex justify-end px-4 pt-4">
			<Button
				variant="outline"
				size="sm"
				disabled={isExporting || runs.length === 0}
				onClick={async () => {
					setIsExporting(true);
					try {
						const runDetails = await Promise.all(
							runs.map((run) =>
								queryClient.fetchQuery(chatDebugRun(chatId, run.id)),
							),
						);
						const exportedAt = new Date();
						const payload = buildChatDebugExport(
							chatId,
							runDetails,
							exportedAt,
						);
						await download(
							buildDebugExportBlob(payload),
							debugExportFilename({ chatId, exportedAt }),
						);
					} catch (error) {
						console.error(error);
						toast.error("Failed to export debug logs.", {
							description: getErrorDetail(error),
						});
					}
					setIsExporting(false);
				}}
			>
				{isExporting ? (
					<Spinner size="sm" loading />
				) : (
					<DownloadIcon className="size-4" />
				)}
				Export debug logs
			</Button>
		</div>
	);
};
