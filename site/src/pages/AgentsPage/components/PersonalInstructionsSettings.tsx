import { useFormik } from "formik";
import type { FC } from "react";
import { useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";

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
	const [isUserPromptOverflowing, setIsUserPromptOverflowing] = useState(false);

	const form = useFormik({
		initialValues: {
			custom_prompt: userPromptData?.custom_prompt ?? "",
		},
		enableReinitialize: true,
		onSubmit: (values, helpers) => {
			onSaveUserPrompt(
				{ custom_prompt: values.custom_prompt },
				{ onSuccess: () => helpers.resetForm() },
			);
		},
	});

	const userInvisibleCharCount = countInvisibleCharacters(
		form.values.custom_prompt,
	);

	return (
		<form className="space-y-2" onSubmit={form.handleSubmit}>
			<h3 className="m-0 text-[13px] font-semibold text-content-primary">
				Personal Instructions
			</h3>
			<p className="!mt-0.5 m-0 text-xs text-content-secondary">
				Applied to all your conversations. Only visible to you.
			</p>
			<TextareaAutosize
				className={cn(
					"max-h-[240px] w-full resize-none rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30",
					isUserPromptOverflowing && "overflow-y-auto [scrollbar-width:thin]",
				)}
				name="custom_prompt"
				placeholder="Additional behavior, style, and tone preferences"
				value={form.values.custom_prompt}
				onChange={form.handleChange}
				onHeightChange={(height) => setIsUserPromptOverflowing(height >= 240)}
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
					onClick={() => form.setFieldValue("custom_prompt", "")}
					disabled={isAnyPromptSaving || !form.values.custom_prompt}
				>
					Clear
				</Button>
				<Button
					size="sm"
					type="submit"
					disabled={isAnyPromptSaving || !form.dirty}
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
