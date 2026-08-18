import { PauseIcon } from "lucide-react";
import type { FC } from "react";
import { Shimmer } from "../ChatElements";
import { ToolIcon } from "../ChatElements/tools/ToolIcon";
import { ChatStatusCallout } from "./ChatStatusCallout";
import type { LiveStatusModel } from "./liveStatusModel";
import { BlockList, type BlockListProps } from "./MessageBlocks";
import { shouldShowGenericThinking } from "./streamingActivity";

const LiveActivitySlot: FC<{ interrupting?: boolean }> = ({
	interrupting = false,
}) => (
	<div
		data-testid="live-activity-slot"
		className="flex h-6 items-center gap-2 text-content-secondary"
	>
		{interrupting ? (
			<PauseIcon className="size-4 shrink-0 stroke-[1.5]" />
		) : (
			<ToolIcon name="thinking" />
		)}
		<Shimmer as="span" className="text-[13px] leading-6">
			{interrupting ? "Interrupting" : "Thinking"}
		</Shimmer>
	</div>
);

type AssistantOutputProps = BlockListProps & {
	// Present only while the turn is still live. Drives the retry/reconnect
	// callout and the generic thinking indicator.
	liveStatus?: LiveStatusModel;
};

/**
 * Renders assistant output from already-normalized blocks and tools, so a live
 * turn and the durable message that replaces it render through the same path.
 */
export const AssistantOutput: FC<AssistantOutputProps> = ({
	liveStatus,
	...blockProps
}) => {
	const { blocks, tools } = blockProps;
	const callout =
		liveStatus?.phase === "retrying" || liveStatus?.phase === "reconnecting"
			? liveStatus
			: undefined;

	return (
		<div className="relative flex flex-col gap-2 overflow-visible">
			<BlockList {...blockProps} />
			{callout && <ChatStatusCallout status={callout} />}
			{liveStatus &&
				(liveStatus.phase === "interrupting" ||
					shouldShowGenericThinking({ liveStatus, blocks, tools })) && (
					<LiveActivitySlot
						interrupting={liveStatus.phase === "interrupting"}
					/>
				)}
		</div>
	);
};
