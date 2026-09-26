import { cn } from "cn";
import { PauseIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { Shimmer } from "../ChatElements/Shimmer";
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
	const showsActivity =
		liveStatus !== undefined &&
		(liveStatus.phase === "interrupting" ||
			shouldShowGenericThinking({ liveStatus, blocks, tools }));

	// Once the activity row has shown under streaming response text, keep its
	// height while that response stays last. Removing it when text resumes
	// shrinks the live row, and the anchored transcript jumps for a frame.
	const canHoldActivityRow =
		liveStatus?.phase === "streaming" && blocks.at(-1)?.type === "response";
	const [holdsActivityRow, setHoldsActivityRow] = useState(false);
	if (showsActivity && canHoldActivityRow && !holdsActivityRow) {
		setHoldsActivityRow(true);
	}
	if (holdsActivityRow && !canHoldActivityRow) {
		setHoldsActivityRow(false);
	}

	let activityRow: ReactNode = null;
	if (showsActivity) {
		activityRow = (
			<LiveActivitySlot interrupting={liveStatus?.phase === "interrupting"} />
		);
	} else if (holdsActivityRow) {
		activityRow = <div aria-hidden className="h-6" />;
	}

	return (
		<div
			className={cn(
				"relative flex flex-col gap-2 overflow-visible",
				// While the turn is live, hold a one-line floor so the Thinking
				// indicator's height survives the handoff to streaming text instead
				// of collapsing for a frame. Once text renders it exceeds the floor,
				// so this is a no-op after the first visible characters. Durable
				// messages carry no liveStatus and are unaffected.
				liveStatus && "min-h-6",
			)}
		>
			<BlockList {...blockProps} />
			{callout && <ChatStatusCallout status={callout} />}
			{activityRow}
		</div>
	);
};
