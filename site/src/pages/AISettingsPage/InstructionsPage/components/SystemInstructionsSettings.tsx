import { useFormik } from "formik";
import type { FC } from "react";
import { useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "#/pages/AgentsPage/components/TemporarySavedState";
import { TextPreviewDialog } from "#/pages/AgentsPage/components/TextPreviewDialog";
import { cn } from "#/utils/cn";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface SystemInstructionsSettingsProps {
	systemPromptData: TypesGen.ChatSystemPromptResponse | undefined;
	onSaveSystemPrompt: (
		req: TypesGen.UpdateChatSystemPromptRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingSystemPrompt: boolean;
	isSaveSystemPromptError: boolean;
	isAnyPromptSaving: boolean;
}

export const SystemInstructionsSettings: FC<
	SystemInstructionsSettingsProps
> = ({
	systemPromptData,
	onSaveSystemPrompt,
	isSavingSystemPrompt,
	isSaveSystemPromptError,
	isAnyPromptSaving,
}) => {
	const [showDefaultPromptPreview, setShowDefaultPromptPreview] =
		useState(false);
	const [isSystemPromptOverflowing, setIsSystemPromptOverflowing] =
		useState(false);
	const { isSavedVisible, showSavedState } = useTemporarySavedState();

	const hasLoadedSystemPrompt = systemPromptData !== undefined;
	const defaultSystemPrompt = systemPromptData?.default_system_prompt ?? "";

	const form = useFormik({
		enableReinitialize: true,
		initialValues: {
			system_prompt: systemPromptData?.system_prompt ?? "",
			include_default_system_prompt:
				systemPromptData?.include_default_system_prompt ?? false,
		},
		onSubmit: (values, { resetForm }) => {
			onSaveSystemPrompt(values, {
				onSuccess: () => {
					showSavedState();
					resetForm();
				},
			});
		},
	});

	const systemInvisibleCharCount = countInvisibleCharacters(
		form.values.system_prompt,
	);
	const isSystemPromptDisabled = isAnyPromptSaving || !hasLoadedSystemPrompt;

	return (
		<>
			<form className="flex flex-col" onSubmit={form.handleSubmit}>
				<div className="flex items-center gap-2 font-sans text-sm font-normal leading-6 text-content-primary">
					<Switch
						checked={form.values.include_default_system_prompt}
						onCheckedChange={(checked) =>
							form.setFieldValue("include_default_system_prompt", checked)
						}
						aria-label="Include Coder Agents default system prompt"
						disabled={isSystemPromptDisabled}
					/>
					<div className="flex min-w-0 items-center gap-1.5">
						<span>Include Coder Agents default system prompt.</span>
						<Button
							size="xs"
							variant="subtle"
							type="button"
							onClick={() => setShowDefaultPromptPreview(true)}
							disabled={!hasLoadedSystemPrompt}
							className="min-w-0 px-0 font-sans text-sm font-normal leading-6 text-content-link hover:text-content-link"
						>
							View prompt
						</Button>
					</div>
				</div>
				<label
					className="mt-4 mb-2 font-sans text-sm font-bold leading-6 text-content-primary"
					htmlFor="system_prompt"
				>
					Additional system instructions
				</label>
				<TextareaAutosize
					className={cn(
						"max-h-[240px] w-full resize-none rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-sm font-normal leading-6 text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30",
						isSystemPromptOverflowing &&
							"overflow-y-auto [scrollbar-width:thin]",
					)}
					id="system_prompt"
					placeholder="Add additional guidance"
					name="system_prompt"
					value={form.values.system_prompt}
					onChange={form.handleChange}
					onHeightChange={(height) =>
						setIsSystemPromptOverflowing(height >= 240)
					}
					disabled={isSystemPromptDisabled}
					minRows={1}
				/>
				{systemInvisibleCharCount > 0 && (
					<Alert severity="warning">
						<AlertDescription>
							This text contains {systemInvisibleCharCount} invisible Unicode{" "}
							{systemInvisibleCharCount !== 1 ? "characters" : "character"} that
							could hide content. These will be stripped on save.
						</AlertDescription>
					</Alert>
				)}
				{(form.dirty || isSavedVisible || isSavingSystemPrompt) && (
					<div className="mt-2 flex min-h-6 justify-end gap-2">
						{isSavedVisible ? (
							<TemporarySavedState />
						) : (
							<>
								<Button
									size="xs"
									variant="outline"
									type="button"
									onClick={() => form.setFieldValue("system_prompt", "")}
									disabled={
										isSystemPromptDisabled || !form.values.system_prompt
									}
								>
									Clear
								</Button>
								<Button
									size="xs"
									type="submit"
									disabled={
										isSystemPromptDisabled ||
										!(form.dirty && hasLoadedSystemPrompt)
									}
								>
									{isSavingSystemPrompt && (
										<Spinner loading className="h-4 w-4" />
									)}
									Save
								</Button>
							</>
						)}
					</div>
				)}
				{isSaveSystemPromptError && (
					<p className="m-0 text-xs text-content-destructive">
						Failed to save system prompt.
					</p>
				)}
			</form>

			{showDefaultPromptPreview && (
				<TextPreviewDialog
					content={defaultSystemPrompt}
					fileName="Default System Prompt"
					onClose={() => setShowDefaultPromptPreview(false)}
				/>
			)}
		</>
	);
};
