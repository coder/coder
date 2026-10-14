import { useState } from "react";
import type { UseMutateFunction } from "react-query";
import TextareaAutosize from "react-textarea-autosize";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { InvisibleCharactersAlert } from "#/components/Alert/InvisibleCharactersAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { Spinner } from "#/components/Spinner/Spinner";
import { toast } from "#/components/Toaster/toast";

type OrganizationInstructionsSettingsProps = {
	systemPrompt: string | undefined;
	isLoading: boolean;
	loadError: unknown;
	refetchError: unknown;
	canEdit: boolean;
	onSave: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateOrganizationChatSystemPromptRequest,
		unknown
	>;
	isSaving: boolean;
	saveError: unknown;
	onResetSave: () => void;
};

export const OrganizationInstructionsSettings: React.FC<
	OrganizationInstructionsSettingsProps
> = ({
	systemPrompt,
	isLoading,
	loadError,
	refetchError,
	canEdit,
	onSave,
	isSaving,
	saveError,
	onResetSave,
}) => {
	// An undefined draft shows the server value, so a background refetch
	// updates an untouched field without discarding in-progress edits.
	const [draft, setDraft] = useState<string>();
	const savedPrompt = systemPrompt ?? "";
	const value = draft ?? savedPrompt;
	const isDirty = draft !== undefined && draft !== savedPrompt;
	const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		onResetSave();
		onSave(
			{ system_prompt: value },
			{
				onSuccess: () => {
					setDraft(undefined);
					toast.success("Organization instructions saved successfully.");
				},
			},
		);
	};

	return (
		<form
			className="flex flex-col rounded-lg border border-solid border-border p-6"
			onSubmit={handleSubmit}
		>
			{loadError != null ? (
				<ErrorAlert error={loadError} />
			) : isLoading ? (
				<Loader label="Loading organization instructions" />
			) : (
				<>
					{refetchError != null && (
						<ErrorAlert error={refetchError} className="mb-4" />
					)}
					<TextareaAutosize
						aria-label="Organization instructions"
						className="w-full resize-none overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary px-4 py-3 font-sans text-sm font-normal leading-6 text-content-primary placeholder:text-content-secondary focus:outline-hidden focus:ring-2 focus:ring-content-link/30 scrollbar-thin"
						placeholder={
							canEdit
								? "Instructions added to new chats in this organization"
								: "No organization instructions"
						}
						value={value}
						onChange={(event) => setDraft(event.target.value)}
						disabled={isSaving}
						readOnly={!canEdit}
						minRows={4}
						maxRows={9}
					/>
					<InvisibleCharactersAlert text={value} className="mt-2" />
					{saveError != null && (
						<p
							role="alert"
							className="m-0 mt-4 text-xs text-content-destructive"
						>
							{getErrorMessage(
								saveError,
								"Failed to save organization instructions.",
							)}
						</p>
					)}
					{canEdit && (
						<div className="mt-6 flex justify-end gap-4">
							<Button
								variant="outline"
								type="button"
								onClick={() => {
									onResetSave();
									setDraft(undefined);
								}}
								disabled={isSaving || (!isDirty && saveError == null)}
							>
								Cancel
							</Button>
							<Button type="submit" disabled={isSaving || !isDirty}>
								{isSaving && <Spinner loading className="size-4" />}
								Save
							</Button>
						</div>
					)}
				</>
			)}
		</form>
	);
};
