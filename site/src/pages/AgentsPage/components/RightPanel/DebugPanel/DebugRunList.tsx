import type { FC } from "react";
import type { ChatDebugRunSummary } from "#/api/typesGenerated";
import { DebugRunCard } from "./DebugRunCard";

interface DebugRunListProps {
	runs: ChatDebugRunSummary[];
	chatId: string;
	enabled?: boolean;
}

export const DebugRunList: FC<DebugRunListProps> = ({
	runs,
	chatId,
	enabled = true,
}) => {
	// Empty state is handled by DebugPanel before rendering this
	// component. No guard here to avoid duplicated copy that drifts.
	return (
		<div className="w-full max-w-full min-w-0">
			{runs.map((run) => (
				<DebugRunCard
					key={run.id}
					run={run}
					chatId={chatId}
					enabled={enabled}
				/>
			))}
		</div>
	);
};
