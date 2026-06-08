import type { FC } from "react";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import {
	ConversationItem,
	Message,
	MessageContent,
	Shimmer,
} from "../ChatElements";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { ToolIcon } from "../ChatElements/tools/ToolIcon";
import { ChatStatusCallout } from "./ChatStatusCallout";
import { BlockList } from "./ConversationTimeline";
import type { LiveStatusModel } from "./liveStatusModel";
import { shouldShowGenericThinking } from "./streamingActivity";
import type { MergedTool, StreamState } from "./types";

const hasTransientLiveStatus = (liveStatus: LiveStatusModel): boolean =>
	liveStatus.phase === "retrying" || liveStatus.phase === "reconnecting";

const LiveActivitySlot: FC<{
	visible: boolean;
	overlay: boolean;
}> = ({ visible, overlay }) => (
	<div
		data-testid="live-activity-slot"
		aria-hidden={!visible}
		className={cn(
			"flex items-center gap-2 text-content-secondary",
			overlay ? "pointer-events-none absolute left-0 top-full mt-2" : "h-6",
			!visible && "invisible",
		)}
	>
		<ToolIcon name="thinking" isError={false} />
		<Shimmer as="span" className="text-[13px] leading-6">
			Thinking
		</Shimmer>
	</div>
);

export const StreamingOutput: FC<{
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles?: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	liveStatus: LiveStatusModel;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}> = ({
	streamState,
	streamTools,
	subagentTitles,
	subagentVariants,
	subagentStatusOverrides,
	liveStatus,
	urlTransform,
	mcpServers,
}) => {
	if (liveStatus.phase === "idle") {
		return null;
	}

	const isStreaming = liveStatus.phase === "streaming";
	const shouldShowBlocks =
		liveStatus.phase === "streaming" || liveStatus.hasAccumulatedOutput;
	const blocks = shouldShowBlocks ? (streamState?.blocks ?? []) : [];

	const showActivity = shouldShowGenericThinking({
		liveStatus,
		streamState,
		streamTools,
	});
	const hasVisibleFlowContent =
		shouldShowBlocks || hasTransientLiveStatus(liveStatus);

	const conversationItemProps = { role: "assistant" as const };

	return (
		<ConversationItem {...conversationItemProps}>
			<Message className="w-full">
				<MessageContent className="whitespace-normal">
					<div className="relative flex flex-col gap-2 overflow-visible">
						{shouldShowBlocks && (
							<BlockList
								blocks={blocks}
								tools={streamTools}
								keyPrefix="stream"
								isStreaming={isStreaming}
								subagentTitles={subagentTitles}
								subagentVariants={subagentVariants}
								subagentStatusOverrides={subagentStatusOverrides}
								urlTransform={urlTransform}
								mcpServers={mcpServers}
							/>
						)}
						{hasTransientLiveStatus(liveStatus) && (
							<ChatStatusCallout status={liveStatus} />
						)}
						<LiveActivitySlot
							visible={showActivity}
							overlay={hasVisibleFlowContent}
						/>
					</div>
				</MessageContent>
			</Message>
		</ConversationItem>
	);
};
