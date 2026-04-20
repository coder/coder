import { SparklesIcon } from "lucide-react";
import { type FC, useEffect, useId, useRef, useState } from "react";
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

export const RenameChatDialog: FC<RenameChatDialogProps> = ({
	chat,
	onRename,
	onPropose,
	onOpenChange,
}) => {
	const [renameTitle, setRenameTitle] = useState("");
	const [isRenamingChat, setIsRenamingChat] = useState(false);
	const [isGeneratingTitle, setIsGeneratingTitle] = useState(false);
	const [generateTitleError, setGenerateTitleError] = useState<string | null>(
		null,
	);
	const inputRef = useRef<HTMLInputElement | null>(null);
	const sessionRef = useRef(0);
	const inputId = useId();
	const errorId = `${inputId}-error`;

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

	useEffect(() => {
		if (prevChatId === null) return;
		sessionRef.current += 1;
	}, [prevChatId]);

	const handleGenerate = async () => {
		if (!chat || !onPropose) return;
		const requestedSession = sessionRef.current;
		setIsGeneratingTitle(true);
		setGenerateTitleError(null);
		try {
			const newTitle = await onPropose(chat.id);
			if (sessionRef.current !== requestedSession) return;
			setRenameTitle(newTitle);
			requestAnimationFrame(() => {
				inputRef.current?.focus();
				inputRef.current?.select();
			});
			setIsGeneratingTitle(false);
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
		const trimmedTitle = renameTitle.trim();
		if (!trimmedTitle) {
			onOpenChange(false);
			return;
		}
		setIsRenamingChat(true);
		await onRename(chat.id, trimmedTitle)
			.then(() => {
				onOpenChange(false);
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
							disabled={isRenamingChat || isGeneratingTitle}
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
							onClick={() => onOpenChange(false)}
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
								isGeneratingTitle
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
