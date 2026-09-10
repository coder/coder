import { cn } from "cn";
import { PauseIcon } from "lucide-react";
import type { FC } from "react";
import { useSlotMachineEnabled } from "../../hooks/useSlotMachineEasterEgg";
import { Shimmer } from "../ChatElements/Shimmer";
import { ToolIcon } from "../ChatElements/tools/ToolIcon";
import { ChatStatusCallout } from "./ChatStatusCallout";
import type { LiveStatusModel } from "./liveStatusModel";
import { BlockList, type BlockListProps } from "./MessageBlocks";
import { SlotMachineIndicator } from "./SlotMachineIndicator";
import { shouldShowGenericThinking } from "./streamingActivity";

/**
 * Live turn status row. Shows the Thinking or Interrupting label. When the
 * slot machine easter egg is enabled the Thinking visual is replaced by
 * `SlotMachineIndicator` while the label stays in the DOM for screen readers.
 */
export const LiveActivitySlot: FC<{ interrupting?: boolean }> = ({
	interrupting = false,
}) => {
	const slotMachine = useSlotMachineEnabled() && !interrupting;
	const label = interrupting ? "Interrupting" : "Thinking";

	return (
		<div
			data-testid="live-activity-slot"
			className="flex h-6 items-center gap-2 text-content-secondary"
		>
			{slotMachine ? (
				<>
					<SlotMachineIndicator />
					<span className="sr-only">{label}</span>
				</>
			) : (
				<>
					{interrupting ? (
						<PauseIcon className="size-4 shrink-0 stroke-[1.5]" />
					) : (
						<ToolIcon name="thinking" />
					)}
					<Shimmer as="span" className="text-[13px] leading-6">
						{label}
					</Shimmer>
				</>
			)}
		</div>
	);
};

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
