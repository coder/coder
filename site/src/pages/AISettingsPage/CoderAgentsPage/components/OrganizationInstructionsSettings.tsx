import { useId, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { Spinner } from "#/components/Spinner/Spinner";
import type { MutationCallbacks } from "#/pages/AISettingsPage/CoderAgentsPage/components/SubagentModelOverrideSettings";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";

type OrganizationInstructionsSettingsProps = {
	systemPrompt: string | undefined;
	isLoading: boolean;
	loadError: unknown;
	canEdit: boolean;
	onSave: (
		req: TypesGen.UpdateOrganizationChatSystemPromptRequest,
		options: MutationCallbacks,
	) => void;
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
	canEdit,
	onSave,
	isSaving,
	saveError,
	onResetSave,
}) => {
	const titleId = useId();
	// An undefined draft shows the server value, so a background refetch
	// updates an untouched field without discarding in-progress edits.
	const [draft, setDraft] = useState<string>();
	const savedPrompt = systemPrompt ?? "";
	const value = draft ?? savedPrompt;
	const isDirty = draft !== undefined && draft !== savedPrompt;
	const invisibleCharCount = countInvisibleCharacters(value);
	const isDisabled = isLoading || isSaving;
	// Treat an error as a load failure only when we have no cached data.
	// A background refetch failure with valid cached data should not hide
	// the form (FE5: keep showing valid data on refetch errors).
	const isLoadError = loadError != null && systemPrompt === undefined;

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
			aria-labelledby={titleId}
			className="flex flex-col rounded-lg border border-solid border-border px-6 py-7"
			onSubmit={handleSubmit}
		>
			<h3
				id={titleId}
				className="m-0 text-sm font-normal leading-6 text-content-primary"
			>
				Organization instructions
			</h3>
			<p className="mt-1 mb-0 text-sm font-normal leading-6 text-content-secondary">
				Added after the deployment instructions when a new chat is created in
				this organization. Existing chats are not affected.
			</p>
			{isLoadError ? (
				<ErrorAlert error={loadError} className="mt-4" />
			) : isLoading ? (
				<Loader label="Loading organization instructions" />
			) : (
				<>
					<TextareaAutosize
						aria-labelledby={titleId}
						className="mt-4 w-full resize-none overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary px-4 py-3 font-sans text-sm font-normal leading-6 text-content-primary placeholder:text-content-secondary focus:outline-hidden focus:ring-2 focus:ring-content-link/30 scrollbar-thin"
						placeholder={
							canEdit
								? "Instructions added to new chats in this organization"
								: "No organization instructions"
						}
						value={value}
						onChange={(event) => setDraft(event.target.value)}
						disabled={isDisabled}
						readOnly={!canEdit}
						minRows={4}
						maxRows={9}
					/>
					{invisibleCharCount > 0 && (
						<Alert severity="warning" className="mt-2">
							<AlertDescription>
								This text contains {invisibleCharCount} invisible Unicode{" "}
								{invisibleCharCount !== 1 ? "characters" : "character"} that
								could hide content. These will be stripped on save.
							</AlertDescription>
						</Alert>
					)}
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
								disabled={isDisabled || (!isDirty && saveError == null)}
							>
								Cancel
							</Button>
							<Button type="submit" disabled={isDisabled || !isDirty}>
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
