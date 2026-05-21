import { UserPenIcon } from "lucide-react";
import { type FC, useEffect, useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link } from "react-router";
import TextareaAutosize from "react-textarea-autosize";
import {
	chatUserCustomPrompt,
	updateUserChatCustomPrompt,
} from "#/api/queries/chats";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "./TemporarySavedState";

interface PersonalInstructionsButtonProps {
	disabled?: boolean;
}

/**
 * A small icon button that opens a popover for editing the current user's
 * personal chat instructions inline, without navigating to the settings
 * page. It reuses the same API and storage as the full settings editor at
 * /agents/settings/general.
 */
export const PersonalInstructionsButton: FC<
	PersonalInstructionsButtonProps
> = ({ disabled = false }) => {
	const queryClient = useQueryClient();
	const userPromptQuery = useQuery(chatUserCustomPrompt());
	const saveMutation = useMutation(updateUserChatCustomPrompt(queryClient));
	const { isSavedVisible, showSavedState } = useTemporarySavedState();

	const [isOpen, setIsOpen] = useState(false);
	const [draft, setDraft] = useState("");
	const [isOverflowing, setIsOverflowing] = useState(false);

	const textareaId = useId();
	const saved = userPromptQuery.data?.custom_prompt ?? "";
	const dirty = draft !== saved;
	const invisibleCharCount = countInvisibleCharacters(draft);

	// Re-seed the draft from the saved value whenever the popover opens or
	// the saved value changes while the popover is closed. This keeps the
	// editor in sync with edits made elsewhere (e.g. the settings page).
	useEffect(() => {
		if (isOpen) {
			setDraft(saved);
		}
	}, [isOpen, saved]);

	const handleOpenChange = (next: boolean) => {
		// Prevent closing while a save is in flight so users see the spinner.
		if (saveMutation.isPending) {
			return;
		}
		setIsOpen(next);
	};

	const handleSave = () => {
		saveMutation.mutate(
			{ custom_prompt: draft },
			{
				onSuccess: () => {
					showSavedState();
					setIsOpen(false);
				},
			},
		);
	};

	return (
		<Popover open={isOpen} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button
					type="button"
					variant="subtle"
					size="icon"
					className="size-7 shrink-0 rounded-full [&>svg]:!size-icon-sm [&>svg]:p-0"
					disabled={disabled}
					aria-label="Edit personal instructions"
				>
					<UserPenIcon />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				side="top"
				align="start"
				className="flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2 p-3"
			>
				<div className="flex flex-col gap-0.5">
					<label
						htmlFor={textareaId}
						className="text-sm font-semibold text-content-primary"
					>
						Personal Instructions
					</label>
					<p className="m-0 text-xs text-content-secondary">
						Applied to all your conversations. Only visible to you.
					</p>
				</div>
				<TextareaAutosize
					id={textareaId}
					className={cn(
						"max-h-[200px] w-full resize-none rounded-md border border-border bg-surface-primary px-3 py-2 font-sans text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link",
						isOverflowing && "overflow-y-auto [scrollbar-width:thin]",
					)}
					placeholder="Additional behavior, style, and tone preferences"
					value={draft}
					onChange={(event) => setDraft(event.target.value)}
					onHeightChange={(height) => setIsOverflowing(height >= 200)}
					minRows={3}
					disabled={saveMutation.isPending || userPromptQuery.isLoading}
				/>
				{invisibleCharCount > 0 && (
					<p className="m-0 text-xs text-content-warning">
						Contains {invisibleCharCount} invisible Unicode{" "}
						{invisibleCharCount === 1 ? "character" : "characters"}. They will
						be stripped on save.
					</p>
				)}
				<div className="mt-1 flex items-center justify-between gap-2">
					<Link
						to="/agents/settings/general"
						className="text-xs text-content-link hover:underline"
						onClick={() => setIsOpen(false)}
					>
						Open full settings
					</Link>
					<div className="flex items-center gap-2">
						{isSavedVisible ? (
							<TemporarySavedState />
						) : (
							<>
								<Button
									size="xs"
									variant="outline"
									type="button"
									onClick={() => setIsOpen(false)}
									disabled={saveMutation.isPending}
								>
									Cancel
								</Button>
								<Button
									size="xs"
									type="button"
									onClick={handleSave}
									disabled={!dirty || saveMutation.isPending}
								>
									{saveMutation.isPending && (
										<Spinner loading className="h-4 w-4" />
									)}
									Save
								</Button>
							</>
						)}
					</div>
				</div>
				{saveMutation.isError && (
					<p className="m-0 text-xs text-content-destructive">
						Failed to save personal instructions.
					</p>
				)}
			</PopoverContent>
		</Popover>
	);
};
