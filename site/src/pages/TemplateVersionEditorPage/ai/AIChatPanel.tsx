import type { AIBridgeModel, AIModelConfig } from "api/queries/aiBridge";
import { Button } from "components/Button/Button";
import { RotateCcwIcon, SparklesIcon, XIcon } from "lucide-react";
import { type FC, useEffect, useRef } from "react";
import type { FileTree } from "utils/filetree";
import { ChatInput } from "./ChatInput";
import { ChatMessage } from "./ChatMessage";
import { ModelConfigBar } from "./ModelConfigBar";
import type { TemplateAgentState } from "./useTemplateAgent";

interface AIChatPanelProps {
	agent: TemplateAgentState;
	getFileTree: () => FileTree;
	modelConfig: AIModelConfig;
	availableModels: readonly AIBridgeModel[];
	onModelConfigChange: (config: AIModelConfig) => void;
	onNavigateToFile?: (path: string) => void;
	onClose: () => void;
}

export const AIChatPanel: FC<AIChatPanelProps> = ({
	agent,
	getFileTree,
	modelConfig,
	availableModels,
	onModelConfigChange,
	onNavigateToFile,
	onClose,
}) => {
	const {
		messages,
		isStreaming,
		status,
		pendingApproval,
		send,
		approve,
		reject,
		reset,
	} = agent;

	const listRef = useRef<HTMLDivElement>(null);

	// Use `messages` directly so the effect re-fires as streaming
	// content grows (the array reference changes on each update).
	useEffect(() => {
		const node = listRef.current;
		if (!node) {
			return;
		}
		if (messages.length === 0 && status === "idle") {
			return;
		}
		node.scrollTo({ top: node.scrollHeight, behavior: "smooth" });
	}, [messages, status]);

	const inputDisabled = isStreaming || status === "awaiting_approval";

	return (
		<div className="flex h-full flex-col border-solid border-0 border-l border-t border-border bg-surface-primary">
			<div className="flex items-center justify-between px-3 py-2">
				<div className="flex items-center gap-2 text-sm font-medium text-content-primary">
					<SparklesIcon className="size-4 text-content-link" />
					<span>AI Assistant</span>
				</div>
				<div className="flex items-center gap-1">
					<Button variant="subtle" size="sm" onClick={reset}>
						<RotateCcwIcon />
						Reset
					</Button>
					<Button
						variant="subtle"
						size="icon"
						onClick={onClose}
						aria-label="Close AI assistant panel"
					>
						<XIcon />
					</Button>
				</div>
			</div>

			<ModelConfigBar
				modelConfig={modelConfig}
				availableModels={availableModels}
				onModelConfigChange={onModelConfigChange}
			/>

			<div ref={listRef} className="flex-1 overflow-y-auto px-3 py-2">
				<div className="flex min-h-full flex-col justify-end gap-3">
					{messages.length === 0 && (
						<p className="m-0 text-xs leading-relaxed text-content-secondary">
							Ask me to inspect or modify your template files. I can read files,
							propose edits, and ask for approval before changing anything.
						</p>
					)}

					{messages.map((message) => (
						<ChatMessage
							key={message.id}
							message={message}
							pendingApproval={pendingApproval}
							onApprove={approve}
							onReject={reject}
							onNavigateToFile={onNavigateToFile}
							getFileTree={getFileTree}
						/>
					))}
				</div>
			</div>

			{isStreaming && (
				<div className="px-3 py-1.5 text-xs text-content-secondary">
					Thinking…
				</div>
			)}

			{status === "error" && (
				<div className="px-3 py-1.5 text-xs text-content-destructive">
					Something went wrong while streaming the assistant response. Reset the
					chat and try again.
				</div>
			)}

			<ChatInput onSend={send} disabled={inputDisabled} />
		</div>
	);
};
