import { X, Play } from "lucide-react";
import type { FC } from "react";
import type { ChatQueuedMessage } from "api/typesGenerated";

interface QueuedMessagesListProps {
	messages: readonly ChatQueuedMessage[];
	onDelete: (id: number) => void;
	onPromote: (id: number) => void;
}

/**
 * Renders a list of queued messages between the streaming output and the
 * chat input. Each message can be deleted (removed from queue) or promoted
 * (interrupt current run and send immediately).
 */
export const QueuedMessagesList: FC<QueuedMessagesListProps> = ({
	messages,
	onDelete,
	onPromote,
}) => {
	if (messages.length === 0) {
		return null;
	}

	return (
		<div className="mx-auto w-full max-w-3xl px-4 pb-2">
			<div className="rounded-lg border border-border-default bg-surface-secondary p-3">
				<div className="mb-2 text-xs font-medium text-content-secondary">
					Queued ({messages.length})
				</div>
				<div className="flex flex-col gap-1.5">
					{messages.map((msg) => (
						<QueuedMessageItem
							key={msg.id}
							message={msg}
							onDelete={onDelete}
							onPromote={onPromote}
						/>
					))}
				</div>
			</div>
		</div>
	);
};

interface QueuedMessageItemProps {
	message: ChatQueuedMessage;
	onDelete: (id: number) => void;
	onPromote: (id: number) => void;
}

const QueuedMessageItem: FC<QueuedMessageItemProps> = ({
	message,
	onDelete,
	onPromote,
}) => {
	// Parse content — the queued message content is a JSON value that
	// could be a plain string, an array of content blocks, or another
	// shape depending on how the user message was serialized.
	let displayText: string;
	try {
		const content = message.content;
		if (typeof content === "string") {
			displayText = content;
		} else if (Array.isArray(content)) {
			// Extract text from content block arrays (e.g. [{type:"text", text:"..."}])
			const texts: string[] = [];
			for (const block of content) {
				if (typeof block === "string") {
					texts.push(block);
				} else if (block && typeof block === "object" && "text" in block) {
					const text = (block as { text: unknown }).text;
					if (typeof text === "string") {
						texts.push(text);
					}
				}
			}
			displayText = texts.join(" ") || JSON.stringify(content);
		} else {
			displayText = JSON.stringify(content);
		}
	} catch {
		displayText = String(message.content);
	}

	return (
		<div className="group flex items-center gap-2 rounded-md bg-surface-primary px-3 py-2">
			<span className="min-w-0 flex-1 truncate text-sm text-content-primary">
				{displayText}
			</span>
			<div className="flex shrink-0 items-center gap-1">
				<button
					type="button"
					onClick={() => onPromote(message.id)}
					className="rounded p-1 text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
					title="Send now"
				>
					<Play className="h-3.5 w-3.5" />
				</button>
				<button
					type="button"
					onClick={() => onDelete(message.id)}
					className="rounded p-1 text-content-secondary hover:bg-surface-tertiary hover:text-content-destructive"
					title="Remove from queue"
				>
					<X className="h-3.5 w-3.5" />
				</button>
			</div>
		</div>
	);
};
