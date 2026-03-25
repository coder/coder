import {
	chatDesktopEnabled,
	chatModelConfigs,
	chatSystemPrompt,
	chatUserCustomPrompt,
	chatWorkspaceTTL,
	updateChatDesktopEnabled,
	updateChatSystemPrompt,
	updateChatWorkspaceTTL,
	updateUserChatCustomPrompt,
} from "api/queries/chats";
import type dayjs from "dayjs";
import { type FC, type FormEvent, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import TextareaAutosize from "react-textarea-autosize";
import { cn } from "utils/cn";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { Switch } from "#/components/Switch/Switch";
import { AdminBadge } from "./components/AdminBadge";
import { ChatModelAdminPanel } from "./components/ChatModelAdminPanel/ChatModelAdminPanel";
import { DurationField } from "./components/DurationField/DurationField";
import { InsightsContent } from "./components/InsightsContent";
import { MCPServerAdminPanel } from "./components/MCPServerAdminPanel";
import { SectionHeader } from "./components/SectionHeader";
import { UsageLimitsTab } from "./components/UsageLimitsTab";
import { UserCompactionThresholdSettings } from "./UserCompactionThresholdSettings";

const textareaMaxHeight = 240;
const textareaBaseClassName =
	"max-h-[240px] w-full resize-none rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30";
const textareaOverflowClassName = "overflow-y-auto [scrollbar-width:thin]";

interface AgentSettingsPageViewProps {
	activeSection: string;
	canManageChatModelConfigs: boolean;
	canSetSystemPrompt: boolean;
	/** Override the current time for date range calculation. Used for
	 *  deterministic Storybook snapshots. */
	now?: dayjs.Dayjs;
}

export const AgentSettingsPageView: FC<AgentSettingsPageViewProps> = ({
	activeSection,
	canManageChatModelConfigs,
	canSetSystemPrompt,
	now,
}) => {
	const queryClient = useQueryClient();

	const systemPromptQuery = useQuery(chatSystemPrompt());
	const {
		mutate: saveSystemPrompt,
		isPending: isSavingSystemPrompt,
		isError: isSaveSystemPromptError,
	} = useMutation(updateChatSystemPrompt(queryClient));

	const userPromptQuery = useQuery(chatUserCustomPrompt());
	const {
		mutate: saveUserPrompt,
		isPending: isSavingUserPrompt,
		isError: isSaveUserPromptError,
	} = useMutation(updateUserChatCustomPrompt(queryClient));

	const desktopEnabledQuery = useQuery(chatDesktopEnabled());
	const {
		mutate: saveDesktopEnabled,
		isPending: isSavingDesktopEnabled,
		isError: isSaveDesktopEnabledError,
	} = useMutation(updateChatDesktopEnabled(queryClient));

	const workspaceTTLQuery = useQuery(chatWorkspaceTTL());
	const modelConfigsQuery = useQuery({
		...chatModelConfigs(),
		enabled: activeSection === "behavior",
	});
	const {
		mutate: saveWorkspaceTTL,
		isPending: isSavingWorkspaceTTL,
		isError: isSaveWorkspaceTTLError,
	} = useMutation(updateChatWorkspaceTTL(queryClient));

	const serverPrompt = systemPromptQuery.data?.system_prompt ?? "";
	const [localEdit, setLocalEdit] = useState<string | null>(null);
	const systemPromptDraft = localEdit ?? serverPrompt;

	const serverUserPrompt = userPromptQuery.data?.custom_prompt ?? "";
	const [localUserEdit, setLocalUserEdit] = useState<string | null>(null);
	const userPromptDraft = localUserEdit ?? serverUserPrompt;

	const [isUserPromptOverflowing, setIsUserPromptOverflowing] = useState(false);
	const [isSystemPromptOverflowing, setIsSystemPromptOverflowing] =
		useState(false);

	const isSystemPromptDirty = localEdit !== null && localEdit !== serverPrompt;
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
	const isPromptSaving = isSavingSystemPrompt || isSavingUserPrompt;
	const isDesktopSaving = isSavingDesktopEnabled;
	const isTTLSaving = isSavingWorkspaceTTL;
	const isTTLLoading = workspaceTTLQuery.isLoading;

	const handleSaveSystemPrompt = (event: FormEvent) => {
		event.preventDefault();
		if (!isSystemPromptDirty) return;
		saveSystemPrompt(
			{ system_prompt: systemPromptDraft },
			{ onSuccess: () => setLocalEdit(null) },
		);
	};

	const handleSaveUserPrompt = (event: FormEvent) => {
		event.preventDefault();
		if (!isUserPromptDirty) return;
		saveUserPrompt(
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
			saveWorkspaceTTL(
				{ workspace_ttl_ms: defaultTTL },
				{ onSuccess: resetAutostopState, onError: resetAutostopState },
			);
		} else {
			setAutostopToggled(false);
			setLocalTTLMs(0);
			saveWorkspaceTTL(
				{ workspace_ttl_ms: 0 },
				{ onSuccess: resetAutostopState, onError: resetAutostopState },
			);
		}
	};

	const handleSaveChatWorkspaceTTL = (event: FormEvent) => {
		event.preventDefault();
		if (!isTTLDirty || isTTLSaving) return;
		saveWorkspaceTTL(
			{ workspace_ttl_ms: localTTLMs ?? 0 },
			{
				onSuccess: resetAutostopState,
				onError: () => setAutostopToggled(null),
			},
		);
	};
	return (
		<div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-4 pt-8 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
			<div className="mx-auto w-full max-w-3xl">
				{activeSection === "behavior" && (
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
							{isSaveUserPromptError && (
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
										<AdminBadge className="ml-auto" />
									</div>
									<p className="!mt-0.5 m-0 text-xs text-content-secondary">
										Applied to all chats for every user. When empty, the
										built-in default is used.
									</p>
									<TextareaAutosize
										className={cn(
											textareaBaseClassName,
											isSystemPromptOverflowing && textareaOverflowClassName,
										)}
										placeholder="Additional behavior, style, and tone preferences for all users"
										value={systemPromptDraft}
										onChange={(event) => setLocalEdit(event.target.value)}
										onHeightChange={(height) =>
											setIsSystemPromptOverflowing(height >= textareaMaxHeight)
										}
										disabled={isPromptSaving}
										minRows={1}
									/>
									<div className="flex justify-end gap-2">
										<Button
											size="sm"
											variant="outline"
											type="button"
											onClick={() => setLocalEdit("")}
											disabled={isPromptSaving || !systemPromptDraft}
										>
											Clear
										</Button>
										<Button
											size="sm"
											type="submit"
											disabled={isPromptSaving || !isSystemPromptDirty}
										>
											Save
										</Button>
									</div>
									{isSaveSystemPromptError && (
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
										<AdminBadge className="ml-auto" />
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
												to be installed in the workspace and the Anthropic
												provider to be configured.
											</p>
											<p className="mt-2 mb-0 font-semibold text-content-secondary">
												Warning: This is a work-in-progress feature, and you’re
												likely to encounter bugs if you enable it.
											</p>
										</div>
										<Switch
											checked={desktopEnabled}
											onCheckedChange={(checked) =>
												saveDesktopEnabled({ enable_desktop: checked })
											}
											aria-label="Enable"
											disabled={isDesktopSaving}
										/>
									</div>
									{isSaveDesktopEnabledError && (
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
										<AdminBadge className="ml-auto" />
									</div>
									<div className="flex items-center justify-between gap-4">
										<p className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
											Set a default autostop for agent-created workspaces that
											don't have one defined in their template. Template-defined
											autostop rules always take precedence. Active chats will
											extend the stop time.
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
													isTTLSaving ||
													!isTTLDirty ||
													isTTLOverMax ||
													isTTLZero
												}
											>
												Save
											</Button>
										</div>
									)}
									{isSaveWorkspaceTTLError && (
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
					</>
				)}
				{activeSection === "providers" && canManageChatModelConfigs && (
					<ChatModelAdminPanel
						section="providers"
						sectionLabel="Providers"
						sectionDescription="Connect third-party LLM services like OpenAI, Anthropic, or Google. Each provider supplies models that users can select for their chats."
						sectionBadge={<AdminBadge className="ml-auto" />}
					/>
				)}
				{activeSection === "models" && canManageChatModelConfigs && (
					<ChatModelAdminPanel
						section="models"
						sectionLabel="Models"
						sectionDescription="Choose which models from your configured providers are available for users to select. You can set a default and adjust context limits."
						sectionBadge={<AdminBadge className="ml-auto" />}
					/>
				)}
				{activeSection === "mcp-servers" && canManageChatModelConfigs && (
					<MCPServerAdminPanel
						sectionLabel="MCP Servers"
						sectionDescription="Configure external MCP servers that provide additional tools for AI chat sessions."
						sectionBadge={<AdminBadge className="ml-auto" />}
					/>
				)}
				{activeSection === "usage" && canManageChatModelConfigs && (
					<UsageLimitsTab now={now} />
				)}
				{activeSection === "insights" && canManageChatModelConfigs && (
					<InsightsContent />
				)}
			</div>
		</div>
	);
};
