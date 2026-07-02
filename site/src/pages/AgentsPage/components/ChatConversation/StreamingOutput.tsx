import type { FC } from "react";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
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

const hasCalloutLiveStatus = (liveStatus: LiveStatusModel): boolean =>
	liveStatus.phase === "retrying" || liveStatus.phase === "reconnecting";

const LiveActivitySlot: FC = () => (
	<div
		data-testid="live-activity-slot"
		className="flex h-6 items-center gap-2 text-content-secondary"
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
						{hasCalloutLiveStatus(liveStatus) && (
							<ChatStatusCallout status={liveStatus} />
						)}
						{showActivity && <LiveActivitySlot />}
					</div>
				</MessageContent>
			</Message>
		</ConversationItem>
	);
};
