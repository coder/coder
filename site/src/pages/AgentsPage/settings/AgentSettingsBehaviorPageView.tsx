import { type FC, type FormEvent, useMemo, useState } from "react";
import type { UseMutationResult, UseQueryResult } from "react-query";
import TextareaAutosize from "react-textarea-autosize";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { Switch } from "#/components/Switch/Switch";
import { cn } from "#/utils/cn";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";
import { AdminBadge } from "../components/AdminBadge";
import { DurationField } from "../components/DurationField/DurationField";
import { SectionHeader } from "../components/SectionHeader";
import { TextPreviewDialog } from "../components/TextPreviewDialog";
import { UserCompactionThresholdSettings } from "../components/UserCompactionThresholdSettings";

const textareaMaxHeight = 240;
const textareaBaseClassName =
	"max-h-[240px] w-full resize-none rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30";
const textareaOverflowClassName = "overflow-y-auto [scrollbar-width:thin]";

interface AgentSettingsBehaviorPageViewProps {
	canSetSystemPrompt: boolean;
	systemPromptQuery: UseQueryResult<TypesGen.ChatSystemPromptResponse>;
	saveSystemPromptMutation: UseMutationResult<
		void,
		unknown,
		TypesGen.UpdateChatSystemPromptRequest
	>;
	userPromptQuery: UseQueryResult<TypesGen.UserChatCustomPrompt>;
	saveUserPromptMutation: UseMutationResult<
		TypesGen.UserChatCustomPrompt,
		unknown,
		TypesGen.UserChatCustomPrompt
	>;
	desktopEnabledQuery: UseQueryResult<TypesGen.ChatDesktopEnabledResponse>;
	saveDesktopEnabledMutation: UseMutationResult<
		void,
		unknown,
		TypesGen.UpdateChatDesktopEnabledRequest
	>;
	workspaceTTLQuery: UseQueryResult<TypesGen.ChatWorkspaceTTLResponse>;
	saveWorkspaceTTLMutation: UseMutationResult<
		void,
		unknown,
		TypesGen.UpdateChatWorkspaceTTLRequest
	>;
	modelConfigsQuery: UseQueryResult<TypesGen.ChatModelConfig[]>;
	compactionThresholdsQuery: UseQueryResult<TypesGen.UserChatCompactionThresholds>;
	updateCompactionThresholdMutation: UseMutationResult<
		TypesGen.UserChatCompactionThreshold,
		unknown,
		{
			modelConfigId: string;
			req: TypesGen.UpdateUserChatCompactionThresholdRequest;
		}
	>;
	deleteCompactionThresholdMutation: UseMutationResult<void, unknown, string>;
}

export const AgentSettingsBehaviorPageView: FC<
	AgentSettingsBehaviorPageViewProps
> = ({
	canSetSystemPrompt,
	systemPromptQuery,
	saveSystemPromptMutation,
	userPromptQuery,
	saveUserPromptMutation,
	desktopEnabledQuery,
	saveDesktopEnabledMutation,
	workspaceTTLQuery,
	saveWorkspaceTTLMutation,
	modelConfigsQuery,
	compactionThresholdsQuery,
	updateCompactionThresholdMutation,
	deleteCompactionThresholdMutation,
}) => {
	const hasLoadedSystemPrompt = systemPromptQuery.isSuccess;
	const serverPrompt = systemPromptQuery.data?.system_prompt ?? "";
	const serverIncludeDefault =
		systemPromptQuery.data?.include_default_system_prompt;
	const defaultSystemPrompt =
		systemPromptQuery.data?.default_system_prompt ?? "";
	const [localEdit, setLocalEdit] = useState<string | null>(null);
	const [localIncludeDefault, setLocalIncludeDefault] = useState<
		boolean | null
	>(null);
	const [showDefaultPromptPreview, setShowDefaultPromptPreview] =
		useState(false);
	const systemPromptDraft = localEdit ?? serverPrompt;
	const includeDefaultDraft =
		localIncludeDefault ?? serverIncludeDefault ?? false;

	const serverUserPrompt = userPromptQuery.data?.custom_prompt ?? "";
	const [localUserEdit, setLocalUserEdit] = useState<string | null>(null);
	const userPromptDraft = localUserEdit ?? serverUserPrompt;

	const systemInvisibleCharCount = useMemo(
		() => countInvisibleCharacters(systemPromptDraft),
		[systemPromptDraft],
	);
	const userInvisibleCharCount = useMemo(
		() => countInvisibleCharacters(userPromptDraft),
		[userPromptDraft],
	);

	const [isUserPromptOverflowing, setIsUserPromptOverflowing] = useState(false);
	const [isSystemPromptOverflowing, setIsSystemPromptOverflowing] =
		useState(false);
	const isSystemPromptDirty =
		hasLoadedSystemPrompt &&
		((localEdit !== null && localEdit !== serverPrompt) ||
			(localIncludeDefault !== null &&
				localIncludeDefault !== serverIncludeDefault));
	const isUserPromptDirty =
		localUserEdit !== null && localUserEdit !== serverUserPrompt;
	const desktopEnabled = desktopEnabledQuery.data?.enable_desktop ?? false;
	const serverTTLMs = workspaceTTLQuery.data?.workspace_ttl_ms ?? 0;
	const [localTTLMs, setLocalTTLMs] = useState<number | null>(null);
	const [autostopToggled, setAutostopToggled] = useState<boolean | null>(null);
	const ttlMs = localTTLMs ?? serverTTLMs;
	const isAutostopEnabled = autostopToggled ?? serverTTLMs > 0;
	const isTTLDirty = localTTLMs !== null && localTTLMs !== serverTTLMs;
	const maxTTLMs = 30 * 24 * 60 * 60_000; // 30 days
	const isTTLOverMax = ttlMs > maxTTLMs;
	const isTTLZero = isAutostopEnabled && ttlMs === 0;
	const isPromptSaving =
		saveSystemPromptMutation.isPending || saveUserPromptMutation.isPending;
	const isSystemPromptDisabled = isPromptSaving || !hasLoadedSystemPrompt;
	const isDesktopSaving = saveDesktopEnabledMutation.isPending;
	const isTTLSaving = saveWorkspaceTTLMutation.isPending;
	const isTTLLoading = workspaceTTLQuery.isLoading;

	const handleSaveSystemPrompt = (event: FormEvent) => {
		event.preventDefault();
		if (!hasLoadedSystemPrompt || !isSystemPromptDirty) return;
		saveSystemPromptMutation.mutate(
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

	const handleSaveUserPrompt = (event: FormEvent) => {
		event.preventDefault();
		if (!isUserPromptDirty) return;
		saveUserPromptMutation.mutate(
			{ custom_prompt: userPromptDraft },
			{ onSuccess: () => setLocalUserEdit(null) },
		);
	};

	const resetAutostopState = () => {
		setLocalTTLMs(null);
		setAutostopToggled(null);
	};

	const handleToggleAutostop = (checked: boolean) => {
		if (checked) {
			// Defensive: restore server value if query cache is
			// stale; otherwise default to 1 hour.
			const defaultTTL = serverTTLMs > 0 ? serverTTLMs : 3_600_000;
			setAutostopToggled(true);
			setLocalTTLMs(defaultTTL);
			saveWorkspaceTTLMutation.mutate(
				{ workspace_ttl_ms: defaultTTL },
				{ onSuccess: resetAutostopState, onError: resetAutostopState },
			);
		} else {
			setAutostopToggled(false);
			setLocalTTLMs(0);
			saveWorkspaceTTLMutation.mutate(
				{ workspace_ttl_ms: 0 },
				{ onSuccess: resetAutostopState, onError: resetAutostopState },
			);
		}
	};

	const handleSaveChatWorkspaceTTL = (event: FormEvent) => {
		event.preventDefault();
		if (!isTTLDirty || isTTLSaving) return;
		saveWorkspaceTTLMutation.mutate(
			{ workspace_ttl_ms: localTTLMs ?? 0 },
			{
				onSuccess: resetAutostopState,
				onError: () => setAutostopToggled(null),
			},
		);
	};

	return (
		<>
			<SectionHeader
				label="Behavior"
				description="Custom instructions that shape how the agent responds in your chats."
			/>
			{/* ── Personal prompt (always visible) ── */}
			<form
				className="space-y-2"
				onSubmit={(event) => void handleSaveUserPrompt(event)}
			>
				<h3 className="m-0 text-[13px] font-semibold text-content-primary">
					Personal Instructions
				</h3>
				<p className="!mt-0.5 m-0 text-xs text-content-secondary">
					Applied to all your chats. Only visible to you.
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
					disabled={isPromptSaving}
					minRows={1}
				/>
				{userInvisibleCharCount > 0 && (
					<Alert severity="warning">
						This text contains {userInvisibleCharCount} invisible Unicode{" "}
						{userInvisibleCharCount !== 1 ? "characters" : "character"} that
						could hide content. These will be stripped on save.
					</Alert>
				)}
				<div className="flex justify-end gap-2">
					<Button
						size="sm"
						variant="outline"
						type="button"
						onClick={() => setLocalUserEdit("")}
						disabled={isPromptSaving || !userPromptDraft}
					>
						Clear
					</Button>
					<Button
						size="sm"
						type="submit"
						disabled={isPromptSaving || !isUserPromptDirty}
					>
						Save
					</Button>
				</div>
				{saveUserPromptMutation.isError && (
					<p className="m-0 text-xs text-content-destructive">
						Failed to save personal instructions.
					</p>
				)}
			</form>

			<hr className="my-5 border-0 border-t border-solid border-border" />
			<UserCompactionThresholdSettings
				modelConfigs={modelConfigsQuery.data ?? []}
				modelConfigsError={modelConfigsQuery.error}
				isLoadingModelConfigs={modelConfigsQuery.isLoading}
				thresholds={compactionThresholdsQuery.data?.thresholds ?? []}
				thresholdsError={compactionThresholdsQuery.error}
				isLoadingThresholds={compactionThresholdsQuery.isLoading}
				updateThresholdMutation={updateCompactionThresholdMutation}
				deleteThresholdMutation={deleteCompactionThresholdMutation}
			/>

			{/* ── Admin system prompt (admin only) ── */}
			{canSetSystemPrompt && (
				<>
					<hr className="my-5 border-0 border-t border-solid border-border" />
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
								This text contains {systemInvisibleCharCount} invisible Unicode{" "}
								{systemInvisibleCharCount !== 1 ? "characters" : "character"}{" "}
								that could hide content. These will be stripped on save.
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
							</Button>
						</div>
						{saveSystemPromptMutation.isError && (
							<p className="m-0 text-xs text-content-destructive">
								Failed to save system prompt.
							</p>
						)}
					</form>
					<hr className="my-5 border-0 border-t border-solid border-border" />
					<div className="space-y-2">
						<div className="flex items-center gap-2">
							<h3 className="m-0 text-[13px] font-semibold text-content-primary">
								Virtual Desktop
							</h3>
							<AdminBadge />
						</div>
						<div className="flex items-center justify-between gap-4">
							<div className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
								<p className="m-0">
									Allow agents to use a virtual, graphical desktop within
									workspaces. Requires the{" "}
									<Link
										href="https://registry.coder.com/modules/coder/portabledesktop"
										target="_blank"
										size="sm"
									>
										portabledesktop module
									</Link>{" "}
									to be installed in the workspace and the Anthropic provider to
									be configured.
								</p>
								<p className="mt-2 mb-0 font-semibold text-content-secondary">
									Warning: This is a work-in-progress feature, and you're likely
									to encounter bugs if you enable it.
								</p>
							</div>
							<Switch
								checked={desktopEnabled}
								onCheckedChange={(checked) =>
									saveDesktopEnabledMutation.mutate({
										enable_desktop: checked,
									})
								}
								aria-label="Enable"
								disabled={isDesktopSaving}
							/>
						</div>
						{saveDesktopEnabledMutation.isError && (
							<p className="m-0 text-xs text-content-destructive">
								Failed to save desktop setting.
							</p>
						)}
					</div>
					<hr className="my-5 border-0 border-t border-solid border-border" />
					<form
						className="space-y-2"
						onSubmit={(event) => void handleSaveChatWorkspaceTTL(event)}
					>
						<div className="flex items-center gap-2">
							<h3 className="m-0 text-[13px] font-semibold text-content-primary">
								Workspace Autostop Fallback
							</h3>
							<AdminBadge />
						</div>
						<div className="flex items-center justify-between gap-4">
							<p className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
								Set a default autostop for agent-created workspaces that don't
								have one defined in their template. Template-defined autostop
								rules always take precedence. Active chats will extend the stop
								time.
							</p>
							<Switch
								checked={isAutostopEnabled}
								onCheckedChange={handleToggleAutostop}
								aria-label="Enable default autostop"
								disabled={isTTLSaving || isTTLLoading}
							/>{" "}
						</div>
						{isAutostopEnabled && (
							<DurationField
								valueMs={ttlMs}
								onChange={(v) => {
									setLocalTTLMs(v);
									// Latch the toggle open while the user is editing
									// so a background refetch cannot unmount the field.
									if (autostopToggled === null) {
										setAutostopToggled(true);
									}
								}}
								label="Autostop Fallback"
								disabled={isTTLSaving || isTTLLoading}
								error={isTTLOverMax || isTTLZero}
								helperText={
									isTTLZero
										? "Duration must be greater than zero."
										: isTTLOverMax
											? "Must not exceed 30 days (720 hours)."
											: undefined
								}
							/>
						)}
						{isAutostopEnabled && (
							<div className="flex justify-end">
								<Button
									size="sm"
									type="submit"
									disabled={
										isTTLSaving || !isTTLDirty || isTTLOverMax || isTTLZero
									}
								>
									Save
								</Button>
							</div>
						)}
						{saveWorkspaceTTLMutation.isError && (
							<p className="m-0 text-xs text-content-destructive">
								Failed to save autostop setting.
							</p>
						)}
						{workspaceTTLQuery.isError && (
							<p className="m-0 text-xs text-content-destructive">
								Failed to load autostop setting.
							</p>
						)}
					</form>
				</>
			)}
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
