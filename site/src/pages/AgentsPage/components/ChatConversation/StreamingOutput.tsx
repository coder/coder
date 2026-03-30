import type { FC } from "react";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { ConversationItem, Message, MessageContent } from "../ChatElements";
import { ChatStatusCallout } from "./ChatStatusCallout";
import { BlockList } from "./ConversationTimeline";
import type { LiveStatusModel } from "./liveStatusModel";
import type { MergedTool, StreamState } from "./types";

const hasTransientLiveStatus = (liveStatus: LiveStatusModel): boolean =>
	liveStatus.phase === "starting" ||
	liveStatus.phase === "retrying" ||
	liveStatus.phase === "reconnecting";

export const StreamingOutput: FC<{
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles?: Map<string, string>;
	computerUseSubagentIds?: Set<string>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	liveStatus: LiveStatusModel;
	startingResetKey?: string;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}> = ({
	streamState,
	streamTools,
	subagentTitles,
	computerUseSubagentIds,
	subagentStatusOverrides,
	liveStatus,
	startingResetKey,
	urlTransform,
	mcpServers,
}) => {
	if (liveStatus.phase === "idle") {
		return null;
	}

	const isStreaming = liveStatus.phase === "streaming";
	const shouldShowBlocks =
		liveStatus.phase === "streaming" || liveStatus.hasAccumulatedOutput;
	const shouldShowStatusCallout = hasTransientLiveStatus(liveStatus);
	if (!shouldShowBlocks && !shouldShowStatusCallout) {
		return null;
	}

	const conversationItemProps = { role: "assistant" as const };
	const blocks = shouldShowBlocks ? (streamState?.blocks ?? []) : [];

	return (
		<ConversationItem {...conversationItemProps}>
			<Message className="w-full">
				<MessageContent className="whitespace-normal">
					<div className="space-y-3">
						{shouldShowBlocks && (
							<BlockList
								blocks={blocks}
								tools={streamTools}
								keyPrefix="stream"
								isStreaming={isStreaming}
								subagentTitles={subagentTitles}
								computerUseSubagentIds={computerUseSubagentIds}
								subagentStatusOverrides={subagentStatusOverrides}
								urlTransform={urlTransform}
								mcpServers={mcpServers}
							/>
						)}
						{shouldShowStatusCallout && (
							<ChatStatusCallout
								status={liveStatus}
								startingResetKey={startingResetKey}
							/>
						)}
					</div>
				</MessageContent>
			</Message>
		</ConversationItem>
	);
};
