import { SparklesIcon } from "lucide-react";
import {
	type FC,
	useEffect,
	useId,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { getErrorMessage } from "#/api/errors";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Input } from "#/components/Input/Input";
import { Spinner } from "#/components/Spinner/Spinner";

type RenameChatDialogProps = {
	readonly chat: Chat | null;
	readonly onRename: (chatId: string, title: string) => Promise<void>;
	readonly onPropose?: (chatId: string) => Promise<string>;
	readonly onOpenChange: (open: boolean) => void;
};

// Generated titles should feel typed without making short titles feel slow.
const GENERATED_TITLE_TYPING_CHARACTERS_PER_SECOND = 80;
const GENERATED_TITLE_TYPING_MS_PER_CHARACTER =
	1000 / GENERATED_TITLE_TYPING_CHARACTERS_PER_SECOND;

const splitGeneratedTitleGraphemes = (title: string): string[] => {
	if (typeof Intl !== "undefined" && typeof Intl.Segmenter === "function") {
		const segmenter = new Intl.Segmenter(undefined, {
			granularity: "grapheme",
		});
		return Array.from(segmenter.segment(title), ({ segment }) => segment);
	}

	return Array.from(title);
};

export const RenameChatDialog: FC<RenameChatDialogProps> = ({
	chat,
	onRename,
	onPropose,
	onOpenChange,
}) => {
	const [renameTitle, setRenameTitle] = useState("");
	const [isRenamingChat, setIsRenamingChat] = useState(false);
	const [isGeneratingTitle, setIsGeneratingTitle] = useState(false);
	const [isTypingGeneratedTitle, setIsTypingGeneratedTitle] = useState(false);
	const [generateTitleError, setGenerateTitleError] = useState<string | null>(
		null,
	);
	const inputRef = useRef<HTMLInputElement | null>(null);
	const generatedTitleTypingFrameRef = useRef<number | null>(null);
	const synchronizedChatIdRef = useRef<string | null | undefined>(undefined);
	const sessionRef = useRef(0);
	const inputId = useId();
	const errorId = `${inputId}-error`;

	const cancelGeneratedTitleTyping = () => {
		if (generatedTitleTypingFrameRef.current !== null) {
			cancelAnimationFrame(generatedTitleTypingFrameRef.current);
			generatedTitleTypingFrameRef.current = null;
		}
		setIsTypingGeneratedTitle(false);
	};

	const finishGeneratedTitleTyping = (
		title: string,
		requestedSession: number,
	) => {
		generatedTitleTypingFrameRef.current = null;
		if (sessionRef.current !== requestedSession) return;

		setRenameTitle(title);
		setIsTypingGeneratedTitle(false);
		generatedTitleTypingFrameRef.current = requestAnimationFrame(() => {
			generatedTitleTypingFrameRef.current = null;
			if (sessionRef.current !== requestedSession) return;
			inputRef.current?.focus();
			inputRef.current?.select();
		});
	};

	const startGeneratedTitleTyping = (
		title: string,
		requestedSession: number,
	) => {
		const graphemes = splitGeneratedTitleGraphemes(title);
		const startedAt = performance.now();

		setRenameTitle("");
		setIsTypingGeneratedTitle(true);
		inputRef.current?.focus();
		inputRef.current?.setSelectionRange(0, 0);

		if (graphemes.length === 0) {
			finishGeneratedTitleTyping(title, requestedSession);
			return;
		}

		const typeNextFrame = (timestamp: number) => {
			if (sessionRef.current !== requestedSession) {
				generatedTitleTypingFrameRef.current = null;
				setIsTypingGeneratedTitle(false);
				return;
			}

			const nextLength = Math.min(
				graphemes.length,
				Math.max(
					1,
					Math.floor(
						(timestamp - startedAt) / GENERATED_TITLE_TYPING_MS_PER_CHARACTER,
					),
				),
			);
			const nextTitle = graphemes.slice(0, nextLength).join("");
			setRenameTitle(nextTitle);

			if (nextLength === graphemes.length) {
				finishGeneratedTitleTyping(title, requestedSession);
				return;
			}

			generatedTitleTypingFrameRef.current =
				requestAnimationFrame(typeNextFrame);
		};

		generatedTitleTypingFrameRef.current = requestAnimationFrame(typeNextFrame);
	};

	const closeDialog = () => {
		sessionRef.current += 1;
		cancelGeneratedTitleTyping();
		setIsGeneratingTitle(false);
		onOpenChange(false);
	};

	const currentChatId = chat?.id ?? null;
	const [prevChatId, setPrevChatId] = useState<string | null>(null);
	if (currentChatId !== prevChatId) {
		setPrevChatId(currentChatId);
		if (chat) {
			setRenameTitle(chat.title);
			setGenerateTitleError(null);
			setIsGeneratingTitle(false);
		}
	}

	useLayoutEffect(() => {
		if (synchronizedChatIdRef.current === prevChatId) return;
		synchronizedChatIdRef.current = prevChatId;
		sessionRef.current += 1;
		if (generatedTitleTypingFrameRef.current !== null) {
			cancelAnimationFrame(generatedTitleTypingFrameRef.current);
			generatedTitleTypingFrameRef.current = null;
		}
		setIsTypingGeneratedTitle(false);
		setIsGeneratingTitle(false);
	});

	useEffect(() => {
		return () => {
			if (generatedTitleTypingFrameRef.current !== null) {
				cancelAnimationFrame(generatedTitleTypingFrameRef.current);
			}
		};
	}, []);

	useEffect(() => {
		if (!isTypingGeneratedTitle) return;
		const input = inputRef.current;
		if (!input) return;
		input.focus();
		const end = renameTitle.length;
		input.setSelectionRange(end, end);
	}, [isTypingGeneratedTitle, renameTitle]);

	const handleGenerate = async () => {
		if (!chat || !onPropose) return;
		const requestedSession = sessionRef.current;
		cancelGeneratedTitleTyping();
		setIsGeneratingTitle(true);
		setGenerateTitleError(null);
		try {
			const newTitle = await onPropose(chat.id);
			if (sessionRef.current !== requestedSession) return;
			setIsGeneratingTitle(false);
			startGeneratedTitleTyping(newTitle, requestedSession);
		} catch (error) {
			if (sessionRef.current !== requestedSession) return;
			setGenerateTitleError(
				getErrorMessage(error, "Failed to generate a new title."),
			);
			setIsGeneratingTitle(false);
		}
	};

	const handleSubmit = async () => {
		if (!chat) return;
		if (isTypingGeneratedTitle) {
			cancelGeneratedTitleTyping();
			return;
		}
		const trimmedTitle = renameTitle.trim();
		if (!trimmedTitle) {
			closeDialog();
			return;
		}
		setIsRenamingChat(true);
		await onRename(chat.id, trimmedTitle)
			.then(() => {
				closeDialog();
			})
			.catch(() => {});
		setIsRenamingChat(false);
	};

	return (
		<Dialog
			open={chat !== null}
			onOpenChange={(open) => {
				// Block closes (escape / outside click) while a rename is in
				// flight; the submit handler will close on success.
				if (!open && isRenamingChat) return;
				if (!open) {
					closeDialog();
					return;
				}
				onOpenChange(open);
			}}
		>
			<DialogContent
				onOpenAutoFocus={(event) => {
					event.preventDefault();
					requestAnimationFrame(() => {
						inputRef.current?.focus();
						inputRef.current?.select();
					});
				}}
				className="max-w-[440px] p-6 sm:p-6"
				aria-describedby={undefined}
			>
				<DialogHeader className="flex-row items-center justify-between space-y-0 sm:flex-row">
					<DialogTitle className="text-lg">Rename chat</DialogTitle>
					{onPropose && (
						<Button
							type="button"
							variant="subtle"
							size="sm"
							className="h-auto min-w-0 gap-1 px-2 py-1.5 text-xs font-normal"
							onClick={() => {
								void handleGenerate();
							}}
							disabled={
								isRenamingChat || isGeneratingTitle || isTypingGeneratedTitle
							}
						>
							{isGeneratingTitle ? (
								<Spinner className="h-[18px] w-[18px]" loading />
							) : (
								<SparklesIcon className="h-[18px] w-[18px]" />
							)}
							Generate
						</Button>
					)}
				</DialogHeader>
				<form
					className="flex flex-col gap-6"
					onSubmit={(event) => {
						event.preventDefault();
						void handleSubmit();
					}}
				>
					<div className="space-y-1.5">
						<Input
							id={inputId}
							ref={inputRef}
							value={renameTitle}
							onChange={(event) => {
								if (isTypingGeneratedTitle) {
									cancelGeneratedTitleTyping();
								}
								setRenameTitle(event.target.value);
								if (generateTitleError) {
									setGenerateTitleError(null);
								}
							}}
							disabled={isRenamingChat || isGeneratingTitle}
							maxLength={200}
							aria-label="Chat title"
							aria-invalid={generateTitleError ? true : undefined}
							aria-describedby={generateTitleError ? errorId : undefined}
						/>
						{generateTitleError && (
							<p
								id={errorId}
								role="alert"
								className="m-0 text-xs text-content-destructive"
							>
								{generateTitleError}
							</p>
						)}
					</div>
					<DialogFooter className="gap-2 sm:space-x-0">
						<Button
							variant="outline"
							size="sm"
							onClick={closeDialog}
							disabled={isRenamingChat}
						>
							Cancel
						</Button>
						<Button
							type="submit"
							size="sm"
							disabled={
								!renameTitle.trim() ||
								renameTitle.trim() === chat?.title ||
								isRenamingChat ||
								isGeneratingTitle ||
								isTypingGeneratedTitle
							}
						>
							{isRenamingChat && <Spinner className="h-4 w-4" loading />}
							Save
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};
