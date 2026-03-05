import { Button } from "components/Button/Button";
import { SendIcon } from "lucide-react";
import {
	type FC,
	type KeyboardEvent,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";

interface ChatInputProps {
	onSend: (text: string) => void;
	disabled?: boolean;
	placeholder?: string;
}

export const ChatInput: FC<ChatInputProps> = ({
	onSend,
	disabled = false,
	placeholder = "Ask about this template...",
}) => {
	const [value, setValue] = useState("");
	const textareaRef = useRef<HTMLTextAreaElement>(null);

	const resizeTextarea = useCallback(() => {
		const textarea = textareaRef.current;
		if (!textarea) {
			return;
		}
		textarea.style.height = "auto";
		textarea.style.height = `${Math.min(textarea.scrollHeight, 150)}px`;
	}, []);

	const handleSend = useCallback(() => {
		if (disabled) {
			return;
		}

		const trimmed = value.trim();
		if (!trimmed) {
			return;
		}

		onSend(trimmed);
		setValue("");
		const textarea = textareaRef.current;
		if (textarea) {
			textarea.style.height = "auto";
		}
	}, [disabled, onSend, value]);

	const onKeyDown = useCallback(
		(event: KeyboardEvent<HTMLTextAreaElement>) => {
			// Ignore Enter during IME composition (e.g., CJK input) so
			// confirming a candidate doesn't send partial text.
			if (event.nativeEvent.isComposing) {
				return;
			}
			if (event.key === "Enter" && !event.shiftKey) {
				event.preventDefault();
				event.stopPropagation();
				handleSend();
			}
		},
		[handleSend],
	);

	useEffect(() => {
		resizeTextarea();
	}, [resizeTextarea]);

	return (
		<div className="flex items-end gap-1.5 px-3 py-2">
			<textarea
				ref={textareaRef}
				value={value}
				onChange={(event) => {
					setValue(event.target.value);
					resizeTextarea();
				}}
				onKeyDown={onKeyDown}
				disabled={disabled}
				rows={1}
				placeholder={placeholder}
				aria-label="Message AI assistant"
				className={cn(
					"max-h-[150px] min-h-10 flex-1 resize-none rounded-md",
					"border border-solid border-border bg-transparent px-3 py-2",
					"text-sm text-content-primary placeholder:text-content-secondary",
					"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
					"disabled:cursor-not-allowed disabled:opacity-50",
				)}
			/>
			<Button
				variant="outline"
				size="icon"
				className="h-10 w-10 shrink-0"
				onClick={handleSend}
				disabled={disabled || value.trim().length === 0}
				aria-label="Send message"
			>
				<SendIcon />
			</Button>
		</div>
	);
};
