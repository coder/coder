import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useLocation } from "react-router";
import { shortRelativeTime } from "#/utils/time";
import { safeBuildAgentChatPath } from "../../../utils/navigation";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

interface ListChatTreeSelf {
	readonly chatId?: string;
	readonly title?: string;
	readonly kind?: string;
}

interface ListChatTreeParent extends ListChatTreeSelf {
	readonly status?: string;
}

interface ListChatTreeChild {
	readonly chatId?: string;
	readonly title?: string;
	readonly status?: string;
	readonly updatedAt?: string;
}

interface ListChatTreeToolProps {
	readonly self?: ListChatTreeSelf;
	/**
	 * Null when the chat has no readable named parent; undefined when the
	 * result carried no parent field at all.
	 */
	readonly parent?: ListChatTreeParent | null;
	readonly childChats: readonly ListChatTreeChild[];
	readonly status: ToolStatus;
	readonly isError: boolean;
	readonly errorMessage?: string;
}

/** Formats a timestamp as a short age, or undefined when it cannot be parsed. */
const relativeAge = (timestamp: string | undefined): string | undefined => {
	if (!timestamp || Number.isNaN(Date.parse(timestamp))) {
		return undefined;
	}
	return shortRelativeTime(timestamp);
};

const ChatRow: FC<{
	readonly chatId?: string;
	readonly title?: string;
	readonly detail: string;
	/** Relative age shown after the detail; excluded from Pixel captures. */
	readonly age?: string;
	readonly emphasis?: boolean;
}> = ({ chatId, title, detail, age, emphasis = false }) => {
	const location = useLocation();
	const path = chatId ? safeBuildAgentChatPath({ chatId }) : null;
	const label = (
		<>
			<span className={emphasis ? "text-content-primary" : undefined}>
				{title?.trim() || "Untitled"}
			</span>
			{(detail || age) && (
				<span className="text-content-secondary/70">
					{" "}
					({detail}
					{detail && age ? ", " : ""}
					{age && <span data-pixel="ignore">{age}</span>})
				</span>
			)}
		</>
	);
	if (!path) {
		return <div className="text-[13px] text-content-secondary">{label}</div>;
	}
	return (
		<Link
			to={{ pathname: path, search: location.search }}
			className="flex w-fit items-center gap-1.5 text-[13px] text-content-secondary opacity-70 transition-opacity hover:opacity-100"
		>
			{label}
			<ExternalLinkIcon className="size-3 shrink-0" />
		</Link>
	);
};

/**
 * Renders a `list_chat_tree` tool call: the calling chat, its parent (or
 * "No parent"), and its unarchived named children with status and age.
 */
export const ListChatTreeTool: FC<ListChatTreeToolProps> = ({
	self,
	parent,
	childChats,
	status,
	isError,
	errorMessage,
}) => {
	const isRunning = status === "running";
	let label: string;
	if (isRunning) {
		label = "Listing chat tree";
	} else if (isError) {
		label = "Failed to list chat tree";
	} else {
		const noun = childChats.length === 1 ? "child" : "children";
		label = `Listed chat tree (${childChats.length} ${noun})`;
	}

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to list the chat tree"}
			hasContent={!isRunning && !isError}
		>
			<ToolCall.Header iconName="list_chat_tree" label={label} />
			<ToolCall.Content>
				<div className="mt-1.5 flex flex-col gap-2 text-[13px] text-content-secondary">
					<div className="flex flex-col gap-0.5">
						<span className="text-xs uppercase tracking-wide text-content-secondary/60">
							Parent
						</span>
						{parent === undefined ? (
							<span>Unknown</span>
						) : parent === null ? (
							<span>No parent</span>
						) : (
							<ChatRow
								chatId={parent.chatId}
								title={parent.title}
								detail={[parent.kind, parent.status]
									.filter((value) => Boolean(value))
									.join(", ")}
							/>
						)}
					</div>
					{self && (
						<div className="flex flex-col gap-0.5">
							<span className="text-xs uppercase tracking-wide text-content-secondary/60">
								This chat
							</span>
							<ChatRow
								chatId={self.chatId}
								title={self.title}
								detail={self.kind ?? "Unknown"}
								emphasis
							/>
						</div>
					)}
					<div className="flex flex-col gap-0.5">
						<span className="text-xs uppercase tracking-wide text-content-secondary/60">
							Children
						</span>
						{childChats.length === 0 ? (
							<span>No active child chats</span>
						) : (
							childChats.map((child, index) => (
								<ChatRow
									key={child.chatId ?? index}
									chatId={child.chatId}
									title={child.title}
									detail={child.status ?? ""}
									age={relativeAge(child.updatedAt)}
								/>
							))
						)}
					</div>
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
