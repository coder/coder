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
	if (runs.length === 0) {
		return (
			<div className="p-4 text-sm text-content-secondary">
				No debug runs recorded yet.
			</div>
		);
	}

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
