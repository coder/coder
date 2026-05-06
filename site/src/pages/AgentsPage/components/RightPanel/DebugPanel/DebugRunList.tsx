import type { FC } from "react";
import type { ChatDebugRunSummary } from "#/api/typesGenerated";
import { DebugRunCard } from "./DebugRunCard";

interface DebugRunListProps {
	runs: ChatDebugRunSummary[];
	chatId: string;
	isVisible: boolean;
	/** Override for the download function; used in tests. */
	download?: (blob: Blob, filename: string) => void;
}

export const DebugRunList: FC<DebugRunListProps> = ({
	runs,
	chatId,
	isVisible,
	download,
}) => {
	// Empty state is handled by DebugPanel before rendering this
	// component. No guard here to avoid duplicated copy that drifts.
	return (
		<div className="w-full max-w-full min-w-0 space-y-3 p-4">
			{runs.map((run) => (
				<DebugRunCard
					key={run.id}
					run={run}
					chatId={chatId}
					isVisible={isVisible}
					download={download}
				/>
			))}
		</div>
	);
};
