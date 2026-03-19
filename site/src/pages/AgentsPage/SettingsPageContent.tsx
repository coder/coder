import { getErrorMessage } from "api/errors";
import {
	chatCostSummary,
	chatCostUsers,
	chatDesktopEnabled,
	chatSystemPrompt,
	chatUserCustomPrompt,
	updateChatDesktopEnabled,
	updateChatSystemPrompt,
	updateUserChatCustomPrompt,
} from "api/queries/chats";
import { userByName } from "api/queries/users";
import type * as TypesGen from "api/typesGenerated";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import { Link } from "components/Link/Link";
import { PaginationAmount } from "components/PaginationWidget/PaginationAmount";
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase";
import { SearchField } from "components/SearchField/SearchField";
import { Spinner } from "components/Spinner/Spinner";
import { Switch } from "components/Switch/Switch";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import dayjs from "dayjs";
import { useDebouncedValue } from "hooks/debounce";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import { ChevronLeftIcon, ShieldIcon } from "lucide-react";
import { type FC, type FormEvent, useCallback, useMemo, useState } from "react";
import {
	keepPreviousData,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useSearchParams } from "react-router";
import TextareaAutosize from "react-textarea-autosize";
import { formatTokenCount } from "utils/analytics";
import { formatCostMicros } from "utils/currency";
import { ChatCostSummaryView } from "./ChatCostSummaryView";
import { ChatModelAdminPanel } from "./ChatModelAdminPanel/ChatModelAdminPanel";
import { InsightsContent } from "./InsightsContent";
import { LimitsTab } from "./LimitsTab";
import { MCPServerAdminPanel } from "./MCPServerAdminPanel";
import { SectionHeader } from "./SectionHeader";

const AdminBadge: FC = () => (
	<TooltipProvider delayDuration={0}>
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="inline-flex cursor-default items-center gap-1 rounded bg-surface-tertiary/60 px-1.5 py-px text-[11px] font-medium text-content-secondary">
					<ShieldIcon className="h-3 w-3" />
					Admin
				</span>
			</TooltipTrigger>
			<TooltipContent side="right">
				Only visible to deployment administrators.
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);

const pageSize = 10;

const UserRow: FC<{
	user: TypesGen.ChatCostUserRollup;
	onSelect: (user: TypesGen.ChatCostUserRollup) => void;
}> = ({ user, onSelect }) => {
	const clickableRowProps = useClickableTableRow({
		onClick: () => onSelect(user),
	});

	return (
		<TableRow
			{...clickableRowProps}
			aria-label={`View details for ${user.name || user.username}`}
		>
			<TableCell className="min-w-[220px] px-4 py-3">
				<AvatarData
					title={user.name || user.username}
					subtitle={`@${user.username}`}
					src={user.avatar_url}
					imgFallbackText={user.username}
				/>
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatCostMicros(user.total_cost_micros)}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{user.message_count.toLocaleString()}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{user.chat_count.toLocaleString()}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatTokenCount(user.total_input_tokens)}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatTokenCount(user.total_output_tokens)}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatTokenCount(user.total_cache_read_tokens)}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatTokenCount(user.total_cache_creation_tokens)}
			</TableCell>
		</TableRow>
	);
};

interface UsageContentProps {
	now?: dayjs.Dayjs;
}

const UsageContent: FC<UsageContentProps> = ({ now }) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const [searchFilter, setSearchFilter] = useState("");
	const debouncedSearch = useDebouncedValue(searchFilter, 300);
	const [page, setPage] = useState(1);
	const dateRange = useMemo(() => {
		const end = now ?? dayjs();
		const start = end.subtract(30, "day");
		return {
			startDate: start.toISOString(),
			endDate: end.toISOString(),
			rangeLabel: `${start.format("MMM D")} – ${end.format("MMM D, YYYY")}`,
		};
	}, [now]);
	const offset = (page - 1) * pageSize;

	const usersQuery = useQuery({
		...chatCostUsers({
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
			username: debouncedSearch || undefined,
			limit: pageSize,
			offset,
		}),
		placeholderData: keepPreviousData,
	});

	const selectedUserId = searchParams.get("user");
	const selectedUserQuery = useQuery({
		...userByName(selectedUserId ?? ""),
		enabled: selectedUserId !== null,
	});
	const selectedUser = selectedUserQuery.data ?? null;

	const summaryQuery = useQuery({
		...chatCostSummary(selectedUserId ?? "me", {
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
		}),
		enabled: selectedUserId !== null,
	});
	const totalCount = usersQuery.data?.count ?? 0;
	const hasPreviousPage = page > 1;
	const hasNextPage = offset + pageSize < totalCount;

	const header = (
		<SectionHeader
			label="Usage"
			description={
				selectedUserId
					? "Review deployment chat usage for a specific user."
					: "Review deployment chat usage and drill into individual users."
			}
			badge={<AdminBadge />}
			action={
				<span className="text-xs text-content-secondary">
					{dateRange.rangeLabel}
				</span>
			}
		/>
	);

	if (selectedUserId) {
		const clearUser = () => {
			setSearchParams((prev) => {
				const next = new URLSearchParams(prev);
				next.delete("user");
				return next;
			});
		};

		const backButton = (
			<button
				type="button"
				onClick={clearUser}
				className="mb-4 inline-flex cursor-pointer items-center gap-0.5 bg-transparent border-0 p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
			>
				{" "}
				<ChevronLeftIcon className="h-4 w-4" />
				Back
			</button>
		);

		if (selectedUserQuery.isLoading) {
			return (
				<div className="space-y-6">
					<div>
						{backButton}
						{header}
					</div>
					<div className="flex min-h-[240px] items-center justify-center">
						<Spinner size="lg" loading className="text-content-secondary" />
					</div>
				</div>
			);
		}

		if (selectedUserQuery.isError || !selectedUser) {
			return (
				<div className="space-y-6">
					<div>
						{backButton}
						{header}
					</div>
					<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
						<p className="m-0 text-sm text-content-secondary">
							{getErrorMessage(
								selectedUserQuery.error,
								"Failed to load user profile.",
							)}
						</p>
						<Button
							variant="outline"
							size="sm"
							type="button"
							onClick={() => void selectedUserQuery.refetch()}
						>
							Retry
						</Button>
					</div>
				</div>
			);
		}

		return (
			<div className="space-y-6">
				<div>
					{backButton}
					{header}
				</div>
				<div className="flex flex-wrap items-center gap-3 rounded-lg border border-border-default bg-surface-secondary px-4 py-3">
					<AvatarData
						title={selectedUser.name || selectedUser.username}
						subtitle={`@${selectedUser.username}`}
						src={selectedUser.avatar_url}
						imgFallbackText={selectedUser.username}
					/>
					<div className="min-w-0 text-xs text-content-secondary">
						<div>User ID: {selectedUser.id}</div>
						<div>{dateRange.rangeLabel}</div>
					</div>
				</div>
				<ChatCostSummaryView
					summary={summaryQuery.data}
					isLoading={summaryQuery.isLoading}
					error={summaryQuery.error}
					onRetry={() => void summaryQuery.refetch()}
					loadingLabel="Loading usage details"
					emptyMessage="No usage data for this user in the selected period."
				/>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{header}
			<div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
				<div className="w-full md:max-w-sm">
					<SearchField
						value={searchFilter}
						onChange={(value) => {
							setSearchFilter(value);
							setPage(1);
						}}
						placeholder="Search by name or username"
						aria-label="Search usage by name or username"
					/>
				</div>
				{usersQuery.data && (
					<PaginationAmount
						limit={pageSize}
						totalRecords={usersQuery.data.count}
						currentOffsetStart={usersQuery.data.count === 0 ? 0 : offset + 1}
						paginationUnitLabel="users"
					/>
				)}
			</div>
			{usersQuery.isLoading && (
				<div
					role="status"
					aria-label="Loading usage"
					className="flex min-h-[240px] items-center justify-center"
				>
					<Spinner size="lg" loading className="text-content-secondary" />
				</div>
			)}

			{usersQuery.error != null && (
				<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
					<p className="m-0 text-sm text-content-secondary">
						{getErrorMessage(usersQuery.error, "Failed to load usage data.")}
					</p>
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={() => void usersQuery.refetch()}
					>
						Retry
					</Button>
				</div>
			)}

			{usersQuery.data &&
				(usersQuery.data.users.length === 0 ? (
					<p className="py-12 text-center text-content-secondary">
						No usage data for this period.
					</p>
				) : (
					<>
						<div className="overflow-hidden rounded-lg border border-border-default">
							<Table>
								<TableHeader>
									<TableRow className="text-left text-xs uppercase tracking-wide text-content-secondary">
										<TableHead className="px-4 py-3">User</TableHead>
										<TableHead className="px-4 py-3 text-right">
											Total Cost
										</TableHead>
										<TableHead className="px-4 py-3 text-right">
											Messages
										</TableHead>
										<TableHead className="px-4 py-3 text-right">
											Chats
										</TableHead>
										<TableHead className="px-4 py-3 text-right">
											Input Tokens
										</TableHead>
										<TableHead className="px-4 py-3 text-right">
											Output Tokens
										</TableHead>
										<TableHead className="px-4 py-3 text-right">
											Cache Read
										</TableHead>
										<TableHead className="px-4 py-3 text-right">
											Cache Write
										</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{usersQuery.data.users.map((user) => (
										<UserRow
											key={user.user_id}
											user={user}
											onSelect={(u) => {
												setSearchParams({ user: u.user_id });
											}}
										/>
									))}
								</TableBody>
							</Table>
						</div>
						<PaginationWidgetBase
							totalRecords={usersQuery.data.count}
							currentPage={page}
							pageSize={pageSize}
							onPageChange={setPage}
							hasPreviousPage={hasPreviousPage}
							hasNextPage={hasNextPage}
						/>
					</>
				))}
		</div>
	);
};

const textareaClassName =
	"max-h-[240px] w-full resize-none overflow-y-auto rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30 [scrollbar-width:thin]";

interface SettingsPageContentProps {
	activeSection: string;
	canManageChatModelConfigs: boolean;
	canSetSystemPrompt: boolean;
	/** Override the current time for date range calculation. Used for
	 *  deterministic Storybook snapshots. */
	now?: dayjs.Dayjs;
}

export const SettingsPageContent: FC<SettingsPageContentProps> = ({
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

	const serverPrompt = systemPromptQuery.data?.system_prompt ?? "";
	const [localEdit, setLocalEdit] = useState<string | null>(null);
	const systemPromptDraft = localEdit ?? serverPrompt;

	const serverUserPrompt = userPromptQuery.data?.custom_prompt ?? "";
	const [localUserEdit, setLocalUserEdit] = useState<string | null>(null);
	const userPromptDraft = localUserEdit ?? serverUserPrompt;

	const isSystemPromptDirty = localEdit !== null && localEdit !== serverPrompt;
	const isUserPromptDirty =
		localUserEdit !== null && localUserEdit !== serverUserPrompt;
	const desktopEnabled = desktopEnabledQuery.data?.enable_desktop ?? false;
	const isDisabled =
		isSavingSystemPrompt || isSavingUserPrompt || isSavingDesktopEnabled;

	const handleSaveSystemPrompt = useCallback(
		(event: FormEvent) => {
			event.preventDefault();
			if (!isSystemPromptDirty) return;
			saveSystemPrompt(
				{ system_prompt: systemPromptDraft },
				{ onSuccess: () => setLocalEdit(null) },
			);
		},
		[isSystemPromptDirty, systemPromptDraft, saveSystemPrompt],
	);

	const handleSaveUserPrompt = useCallback(
		(event: FormEvent) => {
			event.preventDefault();
			if (!isUserPromptDirty) return;
			saveUserPrompt(
				{ custom_prompt: userPromptDraft },
				{ onSuccess: () => setLocalUserEdit(null) },
			);
		},
		[isUserPromptDirty, userPromptDraft, saveUserPrompt],
	);

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
								className={textareaClassName}
								placeholder="Additional behavior, style, and tone preferences"
								value={userPromptDraft}
								onChange={(event) => setLocalUserEdit(event.target.value)}
								disabled={isDisabled}
								minRows={1}
							/>
							<div className="flex justify-end gap-2">
								<Button
									size="sm"
									variant="outline"
									type="button"
									onClick={() => setLocalUserEdit("")}
									disabled={isDisabled || !userPromptDraft}
								>
									Clear
								</Button>
								<Button
									size="sm"
									type="submit"
									disabled={isDisabled || !isUserPromptDirty}
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
									<p className="!mt-0.5 m-0 text-xs text-content-secondary">
										Applied to all chats for every user. When empty, the
										built-in default is used.
									</p>
									<TextareaAutosize
										className={textareaClassName}
										placeholder="Additional behavior, style, and tone preferences for all users"
										value={systemPromptDraft}
										onChange={(event) => setLocalEdit(event.target.value)}
										disabled={isDisabled}
										minRows={1}
									/>
									<div className="flex justify-end gap-2">
										<Button
											size="sm"
											variant="outline"
											type="button"
											onClick={() => setLocalEdit("")}
											disabled={isDisabled || !systemPromptDraft}
										>
											Clear
										</Button>
										<Button
											size="sm"
											type="submit"
											disabled={isDisabled || !isSystemPromptDirty}
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
											disabled={isDisabled}
										/>
									</div>
									{isSaveDesktopEnabledError && (
										<p className="m-0 text-xs text-content-destructive">
											Failed to save desktop setting.
										</p>
									)}
								</div>
							</>
						)}
					</>
				)}
				{activeSection === "providers" && canManageChatModelConfigs && (
					<ChatModelAdminPanel
						section="providers"
						sectionLabel="Providers"
						sectionDescription="Connect third-party LLM services like OpenAI, Anthropic, or Google. Each provider supplies models that users can select for their chats."
						sectionBadge={<AdminBadge />}
					/>
				)}
				{activeSection === "models" && canManageChatModelConfigs && (
					<ChatModelAdminPanel
						section="models"
						sectionLabel="Models"
						sectionDescription="Choose which models from your configured providers are available for users to select. You can set a default and adjust context limits."
						sectionBadge={<AdminBadge />}
					/>
				)}
				{activeSection === "mcp-servers" && canManageChatModelConfigs && (
					<MCPServerAdminPanel
						sectionLabel="MCP Servers"
						sectionDescription="Configure external MCP servers that provide additional tools for AI chat sessions."
						sectionBadge={<AdminBadge />}
					/>
				)}
				{activeSection === "limits" && canManageChatModelConfigs && (
					<LimitsTab />
				)}
				{activeSection === "usage" && canManageChatModelConfigs && (
					<UsageContent now={now} />
				)}
				{activeSection === "insights" && canManageChatModelConfigs && (
					<InsightsContent />
				)}
			</div>
		</div>
	);
};
