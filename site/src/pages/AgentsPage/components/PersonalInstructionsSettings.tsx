import { cn } from "cn";
import { useFormik } from "formik";
import { useId, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { InvisibleCharactersAlert } from "#/components/Alert/InvisibleCharactersAlert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "#/components/TemporarySavedState/TemporarySavedState";

type MutationCallbacks = {
	onSuccess?: (data: TypesGen.UserChatCustomPrompt) => void;
	onError?: () => void;
};

type PersonalInstructionsSettingsProps = {
	userPromptData: TypesGen.UserChatCustomPrompt | undefined;
	isLoadingUserPrompt: boolean;
	userPromptError: unknown;
	onSaveUserPrompt: (
		req: TypesGen.UserChatCustomPrompt,
		options?: MutationCallbacks,
	) => void;
	isSavingUserPrompt: boolean;
	isSaveUserPromptError: boolean;
	isAnyPromptSaving: boolean;
};

export const PersonalInstructionsSettings: React.FC<
	PersonalInstructionsSettingsProps
> = ({
	userPromptData,
	isLoadingUserPrompt,
	userPromptError,
	onSaveUserPrompt,
	isSavingUserPrompt,
	isSaveUserPromptError,
	isAnyPromptSaving,
}) => {
	const headingId = useId();
	const [hasDraft, setHasDraft] = useState(false);
	const hasLoadedPrompt = userPromptData !== undefined;
	const isFormDisabled = isAnyPromptSaving || !hasLoadedPrompt;
	const [isUserPromptOverflowing, setIsUserPromptOverflowing] = useState(false);
	const { isSavedVisible, showSavedState } = useTemporarySavedState();

	const form = useFormik({
		initialValues: {
			custom_prompt: userPromptData?.custom_prompt ?? "",
		},
		enableReinitialize: !hasDraft,
		onSubmit: (values, helpers) => {
			if (isFormDisabled) {
				return;
			}
			onSaveUserPrompt(
				{ custom_prompt: values.custom_prompt },
				{
					onSuccess: (data) => {
						showSavedState();
						helpers.resetForm({ values: data });
						setHasDraft(false);
					},
				},
			);
		},
	});

	const changePrompt = (customPrompt: string) => {
		setHasDraft(customPrompt !== form.initialValues.custom_prompt);
		void form.setFieldValue("custom_prompt", customPrompt);
	};

	return (
		<form className="flex flex-col gap-2" onSubmit={form.handleSubmit}>
			<h3
				id={headingId}
				className="m-0 text-sm font-semibold text-content-primary"
			>
				Personal instructions
			</h3>
			<p className="m-0 text-xs text-content-secondary">
				Applied to all your conversations. Only visible to you.
			</p>
			{!hasLoadedPrompt &&
				(isLoadingUserPrompt ? (
					<div
						role="status"
						className="flex items-center gap-2 text-xs text-content-secondary"
					>
						<Spinner loading className="size-4" />
						Loading personal instructions...
					</div>
				) : userPromptError ? (
					<ErrorAlert error={userPromptError} showDebugDetail={false} />
				) : null)}
			<TextareaAutosize
				aria-labelledby={headingId}
				className={cn(
					"max-h-[240px] w-full resize-none rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-sm leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-hidden focus:ring-2 focus:ring-content-link",
					isUserPromptOverflowing && "overflow-y-auto scrollbar-thin",
				)}
				name="custom_prompt"
				placeholder="Additional behavior, style, and tone preferences"
				value={form.values.custom_prompt}
				onChange={(event) => changePrompt(event.target.value)}
				onHeightChange={(height) => setIsUserPromptOverflowing(height >= 240)}
				disabled={isFormDisabled}
				minRows={1}
			/>
			<InvisibleCharactersAlert text={form.values.custom_prompt} />
			<div className="mt-2 flex min-h-6 justify-end gap-2">
				{(form.dirty || isSavedVisible || isSavingUserPrompt) &&
					(isSavedVisible ? (
						<TemporarySavedState />
					) : (
						<>
							<Button
								size="xs"
								variant="outline"
								type="button"
								onClick={() => changePrompt("")}
								disabled={isFormDisabled || !form.values.custom_prompt}
							>
								Clear
							</Button>
							<Button
								size="xs"
								type="submit"
								disabled={isFormDisabled || !form.dirty}
							>
								{isSavingUserPrompt && <Spinner loading className="size-4" />}
								Save
							</Button>
						</>
					))}
			</div>
			{isSaveUserPromptError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save personal instructions.
				</p>
			)}
		</form>
	);
};
