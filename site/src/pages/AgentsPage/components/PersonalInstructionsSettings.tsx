import type { FC, FormEvent } from "react";
import { useMemo, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";

const textareaMaxHeight = 240;
const textareaBaseClassName =
	"max-h-[240px] w-full resize-none rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30";
const textareaOverflowClassName = "overflow-y-auto [scrollbar-width:thin]";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface PersonalInstructionsSettingsProps {
	userPromptData: TypesGen.UserChatCustomPrompt | undefined;
	onSaveUserPrompt: (
		req: TypesGen.UserChatCustomPrompt,
		options?: MutationCallbacks,
	) => void;
	isSavingUserPrompt: boolean;
	isSaveUserPromptError: boolean;
	isAnyPromptSaving: boolean;
}

export const PersonalInstructionsSettings: FC<
	PersonalInstructionsSettingsProps
> = ({
	userPromptData,
	onSaveUserPrompt,
	isSaveUserPromptError,
	isAnyPromptSaving,
}) => {
	const [localUserEdit, setLocalUserEdit] = useState<string | null>(null);
	const [isUserPromptOverflowing, setIsUserPromptOverflowing] = useState(false);

	const serverUserPrompt = userPromptData?.custom_prompt ?? "";
	const userPromptDraft = localUserEdit ?? serverUserPrompt;
	const userInvisibleCharCount = useMemo(
		() => countInvisibleCharacters(userPromptDraft),
		[userPromptDraft],
	);
	const isUserPromptDirty =
		localUserEdit !== null && localUserEdit !== serverUserPrompt;

	const handleSaveUserPrompt = (event: FormEvent) => {
		event.preventDefault();
		if (!isUserPromptDirty) return;
		onSaveUserPrompt(
			{ custom_prompt: userPromptDraft },
			{ onSuccess: () => setLocalUserEdit(null) },
		);
	};

	return (
		<form
			className="space-y-2"
			onSubmit={(event) => void handleSaveUserPrompt(event)}
		>
			<h3 className="m-0 text-[13px] font-semibold text-content-primary">
				Personal Instructions
			</h3>
			<p className="!mt-0.5 m-0 text-xs text-content-secondary">
				Applied to all your conversations. Only visible to you.
			</p>
			<TextareaAutosize
				className={cn(
					textareaBaseClassName,
					isUserPromptOverflowing && textareaOverflowClassName,
				)}
				placeholder="Additional behavior, style, and tone preferences"
				value={userPromptDraft}
				onChange={(event) => setLocalUserEdit(event.target.value)}
				onHeightChange={(height) =>
					setIsUserPromptOverflowing(height >= textareaMaxHeight)
				}
				disabled={isAnyPromptSaving}
				minRows={1}
			/>
			{userInvisibleCharCount > 0 && (
				<Alert severity="warning">
					<AlertDescription>
						This text contains {userInvisibleCharCount} invisible Unicode{" "}
						{userInvisibleCharCount !== 1 ? "characters" : "character"} that
						could hide content. These will be stripped on save.
					</AlertDescription>
				</Alert>
			)}
			<div className="flex justify-end gap-2">
				<Button
					size="sm"
					variant="outline"
					type="button"
					onClick={() => setLocalUserEdit("")}
					disabled={isAnyPromptSaving || !userPromptDraft}
				>
					Clear
				</Button>
				<Button
					size="sm"
					type="submit"
					disabled={isAnyPromptSaving || !isUserPromptDirty}
				>
					Save
				</Button>
			</div>
			{isSaveUserPromptError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save personal instructions.
				</p>
			)}
		</form>
	);
};
