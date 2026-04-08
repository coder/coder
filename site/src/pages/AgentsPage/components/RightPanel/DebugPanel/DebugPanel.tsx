import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { chatDebugRuns } from "#/api/queries/chats";
import { Alert } from "#/components/Alert/Alert";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Spinner } from "#/components/Spinner/Spinner";
import { DebugRunList } from "./DebugRunList";

interface DebugPanelProps {
	chatId: string;
	enabled?: boolean;
}

export const DebugPanel: FC<DebugPanelProps> = ({
	chatId,
	enabled = false,
}) => {
	const runsQuery = useQuery({
		...chatDebugRuns(chatId),
		enabled,
	});

	const sortedRuns = (runsQuery.data ?? []).toSorted((left, right) => {
		const rightTime = Date.parse(right.started_at || right.updated_at) || 0;
		const leftTime = Date.parse(left.started_at || left.updated_at) || 0;
		return rightTime - leftTime;
	});

	let content: ReactNode;
	if (runsQuery.isError) {
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
			<div className="flex flex-col gap-2 p-4 text-sm text-content-secondary">
				<p className="font-medium text-content-primary">
					No debug runs recorded yet
				</p>
				<p>
					Debug logging captures LLM request/response data for each chat turn,
					title generation, and compaction operation.
				</p>
				<p>
					Enable it from <strong>Settings → Behavior</strong> if your admin
					allows user-controlled debug logging, or ask an admin to turn it on
					globally.
				</p>
			</div>
		);
	} else {
		content = (
			<DebugRunList runs={sortedRuns} chatId={chatId} enabled={enabled} />
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
