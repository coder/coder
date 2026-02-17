import {
	ModelSelector,
	type ModelSelectorOption,
} from "components/ai-elements";
import { Button } from "components/Button/Button";
import { ArrowUpIcon, Loader2Icon, Square } from "lucide-react";
import { memo, type ReactNode, useCallback, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import { formatProviderLabel } from "./modelOptions";

interface AgentChatInputProps {
	onSend: (message: string) => Promise<void>;
	placeholder?: string;
	isDisabled: boolean;
	isLoading: boolean;
	// Optional initial value for the textarea (e.g. restored from
	// localStorage on the create page).
	initialValue?: string;
	// Fires whenever the textarea value changes, useful for persisting
	// the draft externally.
	onInputChange?: (value: string) => void;
	// Model selector.
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	hasModelOptions: boolean;
	// Status messages.
	inputStatusText: string | null;
	modelCatalogStatusMessage: string | null;
	// Streaming controls (optional, for the detail page).
	isStreaming?: boolean;
	onInterrupt?: () => void;
	isInterruptPending?: boolean;
	// Extra controls rendered in the left action area (e.g. workspace
	// selector on the create page).
	leftActions?: ReactNode;
	// When true the entire input sticks to the bottom of the scroll
	// container (used in the detail page).
	sticky?: boolean;
}

export const AgentChatInput = memo<AgentChatInputProps>(
	({
		onSend,
		placeholder = "Type a message...",
		isDisabled,
		isLoading,
		initialValue = "",
		onInputChange,
		selectedModel,
		onModelChange,
		modelOptions,
		modelSelectorPlaceholder,
		hasModelOptions,
		inputStatusText,
		modelCatalogStatusMessage,
		isStreaming = false,
		onInterrupt,
		isInterruptPending = false,
		leftActions,
		sticky = false,
	}) => {
		const [input, setInput] = useState(initialValue);

		const handleSubmit = useCallback(async () => {
			const text = input.trim();
			if (!text || isDisabled || !hasModelOptions) {
				return;
			}
			try {
				await onSend(input);
				setInput("");
				onInputChange?.("");
			} catch {
				// Keep input on failure so the user can retry.
			}
		}, [input, isDisabled, hasModelOptions, onSend, onInputChange]);

		const handleKeyDown = useCallback(
			(e: React.KeyboardEvent) => {
				if (e.key === "Enter" && !e.shiftKey) {
					e.preventDefault();
					void handleSubmit();
				}
			},
			[handleSubmit],
		);

		const content = (
			<div className="mx-auto w-full max-w-3xl pb-4">
				<div className="rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-1 shadow-sm focus-within:ring-2 focus-within:ring-content-link/40">
					<TextareaAutosize
						className="min-h-[120px] w-full resize-none border-none bg-transparent px-3 py-2 font-sans text-[15px] leading-6 text-content-primary outline-none placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-70"
						placeholder={placeholder}
						value={input}
						onChange={(e) => {
							setInput(e.target.value);
							onInputChange?.(e.target.value);
						}}
						onKeyDown={handleKeyDown}
						disabled={isDisabled}
						minRows={4}
					/>
					<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
						<div className="flex min-w-0 items-center gap-2">
							<ModelSelector
								value={selectedModel}
								onValueChange={onModelChange}
								options={modelOptions}
								disabled={isDisabled}
								placeholder={modelSelectorPlaceholder}
								formatProviderLabel={formatProviderLabel}
								dropdownSide="top"
								dropdownAlign="start"
								className="h-8 w-auto justify-start border-none bg-transparent px-1 text-xs shadow-none hover:bg-transparent [&>span]:!text-content-secondary"
							/>
							{leftActions}
							{inputStatusText && (
								<span className="hidden text-xs text-content-secondary sm:inline">
									{inputStatusText}
								</span>
							)}
						</div>
						<div className="flex items-center gap-2">
							{isStreaming && onInterrupt && (
								<Button
									size="icon"
									variant="outline"
									className="rounded-full"
									onClick={onInterrupt}
									disabled={isInterruptPending}
								>
									<Square className="h-4 w-4" />
									<span className="sr-only">Interrupt</span>
								</Button>
							)}
							<Button
								size="icon"
								variant="default"
								className="rounded-full transition-colors [&>svg]:!size-6"
								onClick={() => void handleSubmit()}
								disabled={isDisabled || !hasModelOptions || !input.trim()}
							>
								{isLoading ? (
									<Loader2Icon className="animate-spin" />
								) : (
									<ArrowUpIcon />
								)}
								<span className="sr-only">Send</span>
							</Button>
						</div>
					</div>
					{inputStatusText && (
						<div className="px-2.5 pb-1 text-xs text-content-secondary sm:hidden">
							{inputStatusText}
						</div>
					)}
					{modelCatalogStatusMessage && (
						<div className="px-2.5 pb-1 text-2xs text-content-secondary">
							{modelCatalogStatusMessage}
						</div>
					)}
				</div>
			</div>
		);

		if (sticky) {
			return (
				<div className="sticky bottom-0 z-50 bg-surface-primary">{content}</div>
			);
		}

		return content;
	},
);
AgentChatInput.displayName = "AgentChatInput";
