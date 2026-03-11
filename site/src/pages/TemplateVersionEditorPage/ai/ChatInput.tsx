import { Button } from "components/Button/Button";
import { ImageIcon, SendIcon, SquareIcon, XIcon } from "lucide-react";
import {
	type ChangeEvent,
	type ClipboardEvent,
	type FC,
	type KeyboardEvent,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { toast } from "sonner";
import { cn } from "utils/cn";
import {
	MAX_CHAT_IMAGE_ATTACHMENTS,
	validateImageAttachment,
} from "./attachments";

interface ChatInputSendPayload {
	text: string;
	attachments: readonly File[];
}

interface ChatInputSendResult {
	accepted: boolean;
	error?: string;
}

interface ChatInputProps {
	onSend: (
		payload: ChatInputSendPayload,
	) => ChatInputSendResult | Promise<ChatInputSendResult>;
	onStop?: () => void;
	disabled?: boolean;
	isStreaming?: boolean;
	placeholder?: string;
}

export const ChatInput: FC<ChatInputProps> = ({
	onSend,
	onStop,
	disabled = false,
	isStreaming = false,
	placeholder = "Ask about this template or paste a screenshot...",
}) => {
	const [value, setValue] = useState("");
	const [attachments, setAttachments] = useState<File[]>([]);
	const [isSending, setIsSending] = useState(false);
	const textareaRef = useRef<HTMLTextAreaElement>(null);
	const fileInputRef = useRef<HTMLInputElement>(null);

	const resizeTextarea = useCallback(() => {
		const textarea = textareaRef.current;
		if (!textarea) {
			return;
		}
		textarea.style.height = "auto";
		textarea.style.height = `${Math.min(textarea.scrollHeight, 150)}px`;
	}, []);
	const isStopMode = isStreaming && onStop !== undefined;
	const inputDisabled = disabled || isSending;
	const draftHasContent = value.trim().length > 0 || attachments.length > 0;

	const attachmentPreviews = useMemo(
		() =>
			attachments.map((file) => ({
				file,
				previewUrl:
					typeof URL.createObjectURL === "function"
						? URL.createObjectURL(file)
						: "",
			})),
		[attachments],
	);

	useEffect(() => {
		return () => {
			if (typeof URL.revokeObjectURL !== "function") {
				return;
			}
			for (const { previewUrl } of attachmentPreviews) {
				if (previewUrl.length > 0) {
					URL.revokeObjectURL(previewUrl);
				}
			}
		};
	}, [attachmentPreviews]);

	const addAttachments = useCallback(
		(incoming: readonly File[]) => {
			if (incoming.length === 0) {
				return;
			}
			const nextAttachments: File[] = [];
			let totalAttachments = attachments.length;
			let firstError: string | undefined;
			for (const file of incoming) {
				const validationError = validateImageAttachment(file);
				if (validationError) {
					firstError ??= validationError;
					continue;
				}
				if (totalAttachments >= MAX_CHAT_IMAGE_ATTACHMENTS) {
					firstError ??= `You can attach up to ${MAX_CHAT_IMAGE_ATTACHMENTS} screenshots per message.`;
					continue;
				}
				nextAttachments.push(file);
				totalAttachments += 1;
			}
			if (nextAttachments.length > 0) {
				setAttachments((prev) => [...prev, ...nextAttachments]);
			}
			if (firstError) {
				toast.error(firstError);
			}
		},
		[attachments.length],
	);

	const handleFileSelection = useCallback(
		(event: ChangeEvent<HTMLInputElement>) => {
			addAttachments(Array.from(event.target.files ?? []));
			// Reset so selecting the same file again still fires onChange.
			event.target.value = "";
		},
		[addAttachments],
	);

	const handlePaste = useCallback(
		(event: ClipboardEvent<HTMLTextAreaElement>) => {
			const images = Array.from(event.clipboardData.files).filter((file) =>
				file.type.startsWith("image/"),
			);
			if (images.length === 0) {
				return;
			}
			event.preventDefault();
			addAttachments(images);
		},
		[addAttachments],
	);

	const handleSend = useCallback(async () => {
		if (inputDisabled) {
			return;
		}
		const trimmed = value.trim();
		if (!trimmed && attachments.length === 0) {
			return;
		}
		setIsSending(true);
		try {
			const result = await onSend({ text: trimmed, attachments });
			if (!result.accepted) {
				if (result.error) {
					toast.error(result.error);
				}
				return;
			}
			setValue("");
			setAttachments([]);
			const textarea = textareaRef.current;
			if (textarea) {
				textarea.style.height = "auto";
			}
		} catch (error) {
			toast.error(
				error instanceof Error ? error.message : "Failed to send the message.",
			);
		} finally {
			setIsSending(false);
		}
	}, [attachments, inputDisabled, onSend, value]);

	const handleStop = useCallback(() => {
		onStop?.();
	}, [onStop]);

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
				void handleSend();
			}
		},
		[handleSend],
	);

	useEffect(() => {
		resizeTextarea();
	}, [resizeTextarea]);

	return (
		<div className="flex flex-col gap-2 px-3 py-2">
			{attachmentPreviews.length > 0 && (
				<div className="flex gap-2 overflow-x-auto">
					{attachmentPreviews.map(({ file, previewUrl }, index) => {
						const fileLabel =
							file.name.trim().length > 0
								? file.name
								: `Screenshot ${index + 1}`;
						const fallbackLabel =
							fileLabel.split(".").pop()?.toUpperCase() ?? "FILE";
						return (
							<div
								key={`${fileLabel}-${file.size}-${file.lastModified}-${index}`}
								className="group relative"
								title={fileLabel}
							>
								{previewUrl.length > 0 ? (
									<img
										src={previewUrl}
										alt={fileLabel}
										className="h-16 w-16 rounded-md border border-border object-cover"
									/>
								) : (
									<div className="flex h-16 w-16 items-center justify-center rounded-md border border-border bg-surface-secondary text-[10px] font-medium text-content-secondary">
										{fallbackLabel}
									</div>
								)}
								<button
									type="button"
									onClick={() => {
										setAttachments((prev) =>
											prev.filter((_, itemIndex) => itemIndex !== index),
										);
									}}
									className="absolute -right-2 -top-2 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full border border-border bg-surface-primary text-content-secondary shadow-sm opacity-0 transition-opacity hover:bg-surface-secondary hover:text-content-primary group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
									aria-label={`Remove ${fileLabel}`}
								>
									<XIcon className="h-3.5 w-3.5" />
								</button>
							</div>
						);
					})}
				</div>
			)}

			<div className="flex items-end gap-1.5">
				<input
					ref={fileInputRef}
					type="file"
					multiple
					accept="image/*"
					onChange={handleFileSelection}
					className="hidden"
				/>
				<Button
					variant="outline"
					size="icon"
					className="h-10 w-10 shrink-0"
					onClick={() => fileInputRef.current?.click()}
					disabled={inputDisabled}
					aria-label="Attach screenshots"
				>
					<ImageIcon />
				</Button>
				<textarea
					ref={textareaRef}
					value={value}
					onChange={(event) => {
						setValue(event.target.value);
						resizeTextarea();
					}}
					onPaste={handlePaste}
					onKeyDown={onKeyDown}
					disabled={inputDisabled}
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
					onClick={isStopMode ? handleStop : () => void handleSend()}
					disabled={!isStopMode && (inputDisabled || !draftHasContent)}
					aria-label={isStopMode ? "Stop response" : "Send message"}
				>
					{isStopMode ? <SquareIcon /> : <SendIcon />}
				</Button>
			</div>
		</div>
	);
};
