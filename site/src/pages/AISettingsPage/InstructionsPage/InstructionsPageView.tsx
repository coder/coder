import { useFormik } from "formik";
import type { FC } from "react";
import { useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import { toast } from "sonner";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { TextPreviewDialog } from "#/pages/AgentsPage/components/TextPreviewDialog";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";

const TEXTAREA_MAX_ROWS = 9;

export interface InstructionsPageViewProps {
	systemPromptData: TypesGen.ChatSystemPromptResponse | undefined;
	planModeInstructionsData:
		| TypesGen.ChatPlanModeInstructionsResponse
		| undefined;
	onSaveSystemPrompt: (
		req: TypesGen.UpdateChatSystemPromptRequest,
	) => Promise<void> | void;
	onSavePlanModeInstructions: (
		req: TypesGen.UpdateChatPlanModeInstructionsRequest,
	) => Promise<void> | void;
	onResetSystemPromptSave: () => void;
	onResetPlanModeInstructionsSave: () => void;
	isSaving: boolean;
	isSaveSystemPromptError: boolean;
	isSavePlanModeInstructionsError: boolean;
}

export const InstructionsPageView: FC<InstructionsPageViewProps> = ({
	systemPromptData,
	planModeInstructionsData,
	...formProps
}) => {
	const hasLoadedInstructions =
		systemPromptData !== undefined && planModeInstructionsData !== undefined;

	// Without this gate, Formik would initialize from empty query fallbacks and
	// keep those values after query data loads.
	if (!hasLoadedInstructions) {
		return null;
	}

	return (
		<InstructionsForm
			systemPromptData={systemPromptData}
			planModeInstructionsData={planModeInstructionsData}
			{...formProps}
		/>
	);
};

interface InstructionsFormProps {
	systemPromptData: TypesGen.ChatSystemPromptResponse;
	planModeInstructionsData: TypesGen.ChatPlanModeInstructionsResponse;
	onSaveSystemPrompt: (
		req: TypesGen.UpdateChatSystemPromptRequest,
	) => Promise<void> | void;
	onSavePlanModeInstructions: (
		req: TypesGen.UpdateChatPlanModeInstructionsRequest,
	) => Promise<void> | void;
	onResetSystemPromptSave: () => void;
	onResetPlanModeInstructionsSave: () => void;
	isSaving: boolean;
	isSaveSystemPromptError: boolean;
	isSavePlanModeInstructionsError: boolean;
}

const InstructionsForm: FC<InstructionsFormProps> = ({
	systemPromptData,
	planModeInstructionsData,
	onSaveSystemPrompt,
	onSavePlanModeInstructions,
	onResetSystemPromptSave,
	onResetPlanModeInstructionsSave,
	isSaving,
	isSaveSystemPromptError,
	isSavePlanModeInstructionsError,
}) => {
	const [showDefaultPromptPreview, setShowDefaultPromptPreview] =
		useState(false);
	const defaultSystemPrompt = systemPromptData.default_system_prompt ?? "";
	const initialValues = {
		system_prompt: systemPromptData.system_prompt ?? "",
		include_default_system_prompt:
			systemPromptData.include_default_system_prompt ?? false,
		plan_mode_instructions:
			planModeInstructionsData.plan_mode_instructions ?? "",
	};

	const form = useFormik({
		initialValues,
		onSubmit: async (values, { resetForm, setValues }) => {
			onResetSystemPromptSave();
			onResetPlanModeInstructionsSave();

			try {
				if (
					values.system_prompt !== initialValues.system_prompt ||
					values.include_default_system_prompt !==
						initialValues.include_default_system_prompt
				) {
					await onSaveSystemPrompt({
						system_prompt: values.system_prompt,
						include_default_system_prompt: values.include_default_system_prompt,
					});
				}

				if (
					values.plan_mode_instructions !== initialValues.plan_mode_instructions
				) {
					await onSavePlanModeInstructions({
						plan_mode_instructions: values.plan_mode_instructions,
					});
				}
			} catch (error) {
				await setValues(values, false);
				throw error;
			}

			toast.success("Instructions saved successfully.");
			resetForm({ values });
		},
	});

	const systemInvisibleCharCount = countInvisibleCharacters(
		form.values.system_prompt,
	);
	const planModeInvisibleCharCount = countInvisibleCharacters(
		form.values.plan_mode_instructions,
	);
	const isDisabled = isSaving || form.isSubmitting;

	return (
		<div className="flex max-w-4xl flex-col gap-8">
			<SettingsHeader>
				<SettingsHeaderTitle>Instructions</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Control the system prompts and plan mode instructions used across the
					deployment.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<form
				className="flex flex-col rounded-lg border border-solid border-border p-6"
				onSubmit={form.handleSubmit}
			>
				<div className="flex items-center gap-2 font-sans text-sm font-normal leading-6 text-content-primary">
					<Switch
						checked={form.values.include_default_system_prompt}
						onCheckedChange={(checked) =>
							form.setFieldValue("include_default_system_prompt", checked)
						}
						aria-label="Include Coder Agents default system prompt"
						disabled={isDisabled}
					/>
					<div className="flex min-w-0 items-center gap-1.5">
						<span>Include Coder Agents default system prompt.</span>
						<Button
							size="xs"
							variant="subtle"
							type="button"
							onClick={() => setShowDefaultPromptPreview(true)}
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
					className="w-full resize-none overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary px-4 py-3 font-sans text-sm font-normal leading-6 text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30 [scrollbar-width:thin]"
					id="system_prompt"
					placeholder="Instructions appended to every agent session"
					name="system_prompt"
					value={form.values.system_prompt}
					onChange={form.handleChange}
					disabled={isDisabled}
					minRows={1}
					maxRows={TEXTAREA_MAX_ROWS}
				/>
				{systemInvisibleCharCount > 0 && (
					<Alert severity="warning" className="mt-2">
						<AlertDescription>
							This text contains {systemInvisibleCharCount} invisible Unicode{" "}
							{systemInvisibleCharCount !== 1 ? "characters" : "character"} that
							could hide content. These will be stripped on save.
						</AlertDescription>
					</Alert>
				)}

				<label
					className="mt-8 mb-2 font-sans text-sm font-bold leading-6 text-content-primary"
					htmlFor="plan_mode_instructions"
				>
					Additional plan mode instructions
				</label>
				<TextareaAutosize
					className="w-full resize-none overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary px-4 py-3 font-sans text-sm font-normal leading-6 text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30 [scrollbar-width:thin]"
					id="plan_mode_instructions"
					placeholder="Instructions applied when the agent enters plan mode"
					name="plan_mode_instructions"
					value={form.values.plan_mode_instructions}
					onChange={form.handleChange}
					disabled={isDisabled}
					minRows={4}
					maxRows={TEXTAREA_MAX_ROWS}
				/>
				{planModeInvisibleCharCount > 0 && (
					<Alert severity="warning" className="mt-2">
						<AlertDescription>
							This text contains {planModeInvisibleCharCount} invisible Unicode{" "}
							{planModeInvisibleCharCount !== 1 ? "characters" : "character"}{" "}
							that could hide content. These will be stripped on save.
						</AlertDescription>
					</Alert>
				)}

				{isSaveSystemPromptError && (
					<p className="m-0 mt-4 text-xs text-content-destructive">
						Failed to save system prompt.
					</p>
				)}
				{isSavePlanModeInstructionsError && (
					<p className="m-0 mt-4 text-xs text-content-destructive">
						Failed to save plan mode instructions.
					</p>
				)}

				<div className="mt-8 flex justify-end gap-4">
					<Button
						variant="outline"
						type="button"
						onClick={() => {
							// Save failures leave mutation errors outside Formik state, so
							// both stores must reset before the clean form disables actions.
							onResetSystemPromptSave();
							onResetPlanModeInstructionsSave();
							form.resetForm({ values: initialValues });
						}}
						disabled={
							isDisabled ||
							(!form.dirty &&
								!isSaveSystemPromptError &&
								!isSavePlanModeInstructionsError)
						}
					>
						Cancel
					</Button>
					<Button type="submit" disabled={isDisabled || !form.dirty}>
						{isSaving && <Spinner loading className="h-4 w-4" />}
						Save
					</Button>
				</div>
			</form>

			{showDefaultPromptPreview && (
				<TextPreviewDialog
					content={defaultSystemPrompt}
					fileName="Default System Prompt"
					onClose={() => setShowDefaultPromptPreview(false)}
				/>
			)}
		</div>
	);
};
