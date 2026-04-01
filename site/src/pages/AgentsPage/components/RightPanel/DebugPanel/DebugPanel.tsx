import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { chatDebugRuns } from "#/api/queries/chats";
import { Alert } from "#/components/Alert/Alert";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Spinner } from "#/components/Spinner/Spinner";
import { DebugRunList } from "./DebugRunList";

interface DebugPanelProps {
	chatId: string;
}

const getErrorMessage = (error: unknown): string => {
	if (error instanceof Error && error.message) {
		return error.message;
	}
	return "Unable to load debug panel data.";
};

export const DebugPanel: FC<DebugPanelProps> = ({ chatId }) => {
	const runsQuery = useQuery(chatDebugRuns(chatId));

	const sortedRuns = [...(runsQuery.data ?? [])].sort(
		(left, right) =>
			Date.parse(right.started_at || right.updated_at) -
			Date.parse(left.started_at || left.updated_at),
	);

	let content: ReactNode;
	if (runsQuery.isError) {
		content = (
			<div className="p-4">
				<Alert severity="error" prominent>
					<p className="text-sm text-content-primary">
						{getErrorMessage(runsQuery.error)}
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
			<div className="p-4 text-sm text-content-secondary">
				No debug runs recorded yet.
			</div>
		);
	} else {
		content = <DebugRunList runs={sortedRuns} chatId={chatId} />;
	}

	return (
		<ScrollArea
			className="h-full"
			viewportClassName="h-full [&>div]:!block [&>div]:!w-full"
		>
			<div className="min-h-full w-full min-w-0 overflow-x-hidden">
				{content}
			</div>
		</ScrollArea>
	);
};
