import type { FC, FormEvent } from "react";
import { useMemo, useState } from "react";
import TextareaAutosize from "react-textarea-autosize";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Switch } from "#/components/Switch/Switch";
import { cn } from "#/utils/cn";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";
import { AdminBadge } from "./AdminBadge";
import { TextPreviewDialog } from "./TextPreviewDialog";

const textareaMaxHeight = 240;
const textareaBaseClassName =
	"max-h-[240px] w-full resize-none rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30";
const textareaOverflowClassName = "overflow-y-auto [scrollbar-width:thin]";

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
	isSavingSystemPrompt: _isSavingSystemPrompt,
	isSaveSystemPromptError,
	isAnyPromptSaving,
}) => {
	const [localEdit, setLocalEdit] = useState<string | null>(null);
	const [localIncludeDefault, setLocalIncludeDefault] = useState<
		boolean | null
	>(null);
	const [showDefaultPromptPreview, setShowDefaultPromptPreview] =
		useState(false);
	const [isSystemPromptOverflowing, setIsSystemPromptOverflowing] =
		useState(false);

	const hasLoadedSystemPrompt = systemPromptData !== undefined;
	const serverPrompt = systemPromptData?.system_prompt ?? "";
	const serverIncludeDefault = systemPromptData?.include_default_system_prompt;
	const defaultSystemPrompt = systemPromptData?.default_system_prompt ?? "";
	const systemPromptDraft = localEdit ?? serverPrompt;
	const includeDefaultDraft =
		localIncludeDefault ?? serverIncludeDefault ?? false;

	const systemInvisibleCharCount = useMemo(
		() => countInvisibleCharacters(systemPromptDraft),
		[systemPromptDraft],
	);

	const isSystemPromptDirty =
		hasLoadedSystemPrompt &&
		((localEdit !== null && localEdit !== serverPrompt) ||
			(localIncludeDefault !== null &&
				localIncludeDefault !== serverIncludeDefault));
	const isSystemPromptDisabled = isAnyPromptSaving || !hasLoadedSystemPrompt;

	const handleSaveSystemPrompt = (event: FormEvent) => {
		event.preventDefault();
		if (!hasLoadedSystemPrompt || !isSystemPromptDirty) return;
		onSaveSystemPrompt(
			{
				system_prompt: systemPromptDraft,
				include_default_system_prompt: includeDefaultDraft,
			},
			{
				onSuccess: () => {
					setLocalEdit(null);
					setLocalIncludeDefault(null);
				},
			},
		);
	};

	return (
		<>
			<form
				className="space-y-2"
				onSubmit={(event) => void handleSaveSystemPrompt(event)}
			>
				<div className="flex items-center gap-2">
					<h3 className="m-0 text-[13px] font-semibold text-content-primary">
						System Instructions
					</h3>
					<AdminBadge />
				</div>
				<div className="flex items-center justify-between gap-4">
					<div className="flex min-w-0 items-center gap-2 text-xs font-medium text-content-primary">
						<span>Include Coder Agents default system prompt</span>
						<Button
							size="xs"
							variant="subtle"
							type="button"
							onClick={() => setShowDefaultPromptPreview(true)}
							disabled={!hasLoadedSystemPrompt}
							className="min-w-0 px-0 text-content-link hover:text-content-link"
						>
							Preview
						</Button>
					</div>
					<Switch
						checked={includeDefaultDraft}
						onCheckedChange={setLocalIncludeDefault}
						aria-label="Include Coder Agents default system prompt"
						disabled={isSystemPromptDisabled}
					/>
				</div>
				<p className="!mt-0.5 m-0 text-xs text-content-secondary">
					{includeDefaultDraft
						? "The built-in Coder Agents prompt is prepended. Additional instructions below are appended."
						: "Only the additional instructions below are used. When empty, no deployment-wide system prompt is sent."}
				</p>
				<TextareaAutosize
					className={cn(
						textareaBaseClassName,
						isSystemPromptOverflowing && textareaOverflowClassName,
					)}
					placeholder="Additional instructions for all users"
					value={systemPromptDraft}
					onChange={(event) => setLocalEdit(event.target.value)}
					onHeightChange={(height) =>
						setIsSystemPromptOverflowing(height >= textareaMaxHeight)
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
				<div className="flex justify-end gap-2">
					<Button
						size="sm"
						variant="outline"
						type="button"
						onClick={() => setLocalEdit("")}
						disabled={isSystemPromptDisabled || !systemPromptDraft}
					>
						Clear
					</Button>
					<Button
						size="sm"
						type="submit"
						disabled={isSystemPromptDisabled || !isSystemPromptDirty}
					>
						Save
					</Button>{" "}
				</div>
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
