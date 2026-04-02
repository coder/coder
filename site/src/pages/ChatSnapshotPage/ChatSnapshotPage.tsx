import { MessageSquarePlusIcon } from "lucide-react";
import { type FC, useCallback, useState } from "react";
import { useQuery } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorStatus, isApiError } from "#/api/errors";
import type {
	ChatMessage,
	ChatMessagePart,
	ChatSharedSnapshot,
	ChatStatus,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { CoderIcon } from "#/components/Icons/CoderIcon";
import { Loader } from "#/components/Loader/Loader";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "#/components/StatusIndicator/StatusIndicator";
import { cn } from "#/utils/cn";
import { relativeTime } from "#/utils/time";
import {
	Conversation,
	ConversationItem,
} from "../AgentsPage/components/ChatElements/Conversation";
import {
	Message,
	MessageContent,
} from "../AgentsPage/components/ChatElements/Message";
import { Response } from "../AgentsPage/components/ChatElements/Response";

const statusVariantMap: Record<ChatStatus, StatusIndicatorProps["variant"]> = {
	running: "success",
	completed: "success",
	error: "failed",
	pending: "pending",
	waiting: "pending",
	paused: "inactive",
};

const ChatSnapshotPage: FC = () => {
	const { token } = useParams() as { token: string };
	const navigate = useNavigate();

	const {
		data: snapshot,
		error,
		isLoading,
	} = useQuery({
		queryKey: ["chatSnapshot", token],
		queryFn: () => API.getChatSharedSnapshot(token),
		retry: false,
	});

	const [isForking, setIsForking] = useState(false);

	const handleContinue = useCallback(async () => {
		setIsForking(true);
		try {
			const result = await API.forkChatFromSnapshot(token);
			navigate(`/agents/${result.chat.id}`);
		} catch (err) {
			if (isApiError(err) && err.response.status === 401) {
				// Not logged in — redirect to login, then back here.
				const returnTo = encodeURIComponent(window.location.pathname);
				window.location.href = `/login?redirect=${returnTo}`;
				return;
			}
			toast.error("Failed to continue conversation");
		} finally {
			setIsForking(false);
		}
	}, [token, navigate]);

	if (isLoading) {
		return (
			<SnapshotShell>
				<Loader className="h-full" />
			</SnapshotShell>
		);
	}

	if (error) {
		const status = getErrorStatus(error);
		let title = "Something went wrong";
		let description =
			"An unexpected error occurred while loading this snapshot.";

		if (status === 404) {
			title = "Snapshot not found";
			description =
				"The snapshot you are looking for does not exist or the link is invalid.";
		} else if (status === 410) {
			title = "Snapshot expired";
			description = "This snapshot has expired and is no longer available.";
		}

		return (
			<SnapshotShell>
				<div className="flex flex-1 items-center justify-center">
					<div className="flex flex-col items-center text-center">
						<h2 className="m-0 text-lg font-medium text-content-primary">
							{title}
						</h2>
						<p className="m-0 mt-1 text-sm text-content-secondary">
							{description}
						</p>
					</div>
				</div>
			</SnapshotShell>
		);
	}

	if (!snapshot) {
		return null;
	}

	return (
		<SnapshotShell>
			{/* Header */}
			<header className="flex items-center justify-between border-0 border-b border-solid border-border px-6 py-4">
				<div className="flex flex-col gap-1">
					<h1 className="m-0 text-base font-semibold text-content-primary">
						{snapshot.chat_title || "Untitled conversation"}
					</h1>
					<span className="text-xs text-content-secondary">
						Chat Snapshot
					</span>
				</div>
				<div className="flex items-center gap-3">
					<Button
						variant="default"
						size="sm"
						onClick={handleContinue}
						disabled={isForking}
					>
						{isForking ? (
							<Spinner size="sm" loading />
						) : (
							<MessageSquarePlusIcon className="size-4" />
						)}
						Continue this conversation
					</Button>
					<StatusIndicator
						variant={statusVariantMap[snapshot.chat_status]}
					>
						<StatusIndicatorDot />
						<span className="[&:first-letter]:uppercase">
							{snapshot.chat_status}
						</span>
					</StatusIndicator>
				</div>
			</header>

			{/* Body */}
			<ScrollArea className="flex-1">
				<div className="mx-auto w-full max-w-screen-md px-6 py-6">
					<SnapshotConversation snapshot={snapshot} />
				</div>
			</ScrollArea>

			{/* Footer */}
			<footer className="flex items-center justify-between border-0 border-t border-solid border-border px-6 py-3">
				<span className="text-xs text-content-secondary">
					Snapshot taken {relativeTime(snapshot.snapshot_at)}
				</span>
				<a
					href="https://coder.com"
					target="_blank"
					rel="noopener noreferrer"
					className="flex items-center gap-1.5 text-xs text-content-secondary no-underline hover:text-content-primary"
				>
					Powered by
					<CoderIcon className="h-4 w-auto" />
				</a>
			</footer>
		</SnapshotShell>
	);
};

export default ChatSnapshotPage;

const SnapshotShell: FC<{ children: React.ReactNode }> = ({ children }) => {
	return (
		<div className="flex h-screen flex-col bg-surface-primary">
			{children}
		</div>
	);
};

/**
 * Extracts the concatenated text content from a message's parts.
 */
function getMessageText(parts: readonly ChatMessagePart[]): string {
	return parts
		.filter((p) => p.type === "text")
		.map((p) => p.text ?? "")
		.join("");
}

type SnapshotConversationProps = {
	snapshot: ChatSharedSnapshot;
};

/**
 * Renders the chat messages using the existing Conversation/Message/Response
 * component primitives from the Agents UI. Only user and assistant messages
 * are shown (system and tool messages are filtered out for readability).
 */
const SnapshotConversation: FC<SnapshotConversationProps> = ({ snapshot }) => {
	const visibleMessages = snapshot.messages.filter(
		(m: ChatMessage) => m.role === "user" || m.role === "assistant",
	);

	if (visibleMessages.length === 0) {
		return (
			<p className="text-sm text-content-secondary m-0">
				No messages in this snapshot.
			</p>
		);
	}

	return (
		<Conversation>
			{visibleMessages.map((msg: ChatMessage) => {
				const text = getMessageText(msg.content ?? []);
				if (!text.trim()) return null;

				return (
					<ConversationItem
						key={msg.id}
						role={msg.role as "user" | "assistant"}
					>
						{msg.role === "user" ? (
							<Message
								className={cn(
									"max-w-[85%] rounded-lg bg-surface-secondary px-4 py-3",
								)}
							>
								<MessageContent>{text}</MessageContent>
							</Message>
						) : (
							<div className="w-full min-w-0">
								<Response>{text}</Response>
							</div>
						)}
					</ConversationItem>
				);
			})}
		</Conversation>
	);
};
