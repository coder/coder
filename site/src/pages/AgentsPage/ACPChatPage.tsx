import {
	MessageScroller,
	useMessageScroller,
} from "@shadcn/react/message-scroller";
import { useRef } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link, useParams } from "react-router";
import {
	acpSession,
	acpSessionPath,
	interruptACPSession,
	sendACPMessage,
} from "#/api/queries/acp";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { acpTranscript } from "./components/ACP/transcript";
import { useACPSession } from "./components/ACP/useACPSession";
import {
	AgentChatInput,
	type ChatMessageInputRef,
} from "./components/AgentChatInput";
import { ConversationTimeline } from "./components/ChatConversation/ConversationTimeline";
import { ChatMessageScroller } from "./components/ChatMessageScroller";
import { ChatTopBar } from "./components/ChatTopBar";

function ACPChatPageContent() {
	const { scrollToEnd } = useMessageScroller();
	const { agentId = "", workspaceAgentId = "", sessionId = "" } = useParams();
	const path = acpSessionPath(agentId, workspaceAgentId, sessionId);
	const query = useACPSession(path);
	const client = useQueryClient();
	const refresh = () => client.invalidateQueries(acpSession(path));
	const send = useMutation({ ...sendACPMessage(path), onSettled: refresh });
	const interrupt = useMutation({
		...interruptACPSession(path),
		onSettled: refresh,
	});
	const input = useRef<ChatMessageInputRef>(null);
	const session = query.data;
	const running =
		session?.status === "running" || session?.status === "interrupting";
	const messages = session ? acpTranscript(session) : [];
	return (
		<div className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
			<ChatTopBar
				externalChat={{
					title: session?.title ?? "ACP agent",
					parentChatId: agentId,
					agentName: session
						? session.agent === "claude_code"
							? "Claude Code"
							: "Codex"
						: undefined,
				}}
				panel={{ showSidebarPanel: false, onToggleSidebar: () => {} }}
			/>
			{query.error && <ErrorAlert error={query.error} />}
			{send.error && <ErrorAlert error={send.error} />}
			{interrupt.error && <ErrorAlert error={interrupt.error} />}
			{query.isLoading && <Spinner loading className="m-4" />}
			{session === null ? (
				<div className="p-6 text-sm text-content-secondary">
					Session expired. Its history disappeared when the workspace agent
					stopped. <Link to={`/agents/${agentId}`}>Back to parent chat</Link>
				</div>
			) : (
				<>
					{session?.error && (
						<div
							role="alert"
							className="px-4 py-2 text-sm text-content-destructive"
						>
							{session.error}
						</div>
					)}
					<ChatMessageScroller
						hasMoreMessages={false}
						isFetchingMoreMessages={false}
						isHydratingMessages={false}
						hasFetchMoreError={false}
						hasTranscriptRows={messages.length > 0}
						onFetchMoreMessages={async () => {}}
					>
						<ConversationTimeline
							organizationId={undefined}
							parsedMessages={messages}
							subagentTitles={new Map()}
							isChatCompleted={!running}
							hasActiveStream={running}
						/>
					</ChatMessageScroller>
					<div className="shrink-0 overflow-y-auto px-4 pb-3 md:pb-0 scrollbar-gutter-stable scrollbar-thin">
						<AgentChatInput
							messageOnly
							inputRef={input}
							onSend={(message) =>
								send.mutate(
									{ message },
									{
										onSuccess: () => {
											scrollToEnd({ behavior: "smooth" });
											if (input.current?.getValue()?.trim() === message.trim())
												input.current.clear();
										},
									},
								)
							}
							isDisabled={!session || !query.connected}
							isLoading={send.isPending}
							isStreaming={running}
							onInterrupt={() => interrupt.mutate()}
							isInterruptPending={interrupt.isPending}
							selectedModel=""
							onModelChange={() => {}}
							modelOptions={[]}
							modelSelectorPlaceholder=""
							hasModelOptions
							canConfigureAgentSetup={false}
							placeholder="Message this agent..."
						/>
					</div>
				</>
			)}
		</div>
	);
}

export default function ACPChatPage() {
	const { workspaceAgentId, sessionId } = useParams();
	return (
		<MessageScroller.Provider
			key={`${workspaceAgentId}/${sessionId}`}
			autoScroll
			defaultScrollPosition="end"
		>
			<ACPChatPageContent />
		</MessageScroller.Provider>
	);
}
