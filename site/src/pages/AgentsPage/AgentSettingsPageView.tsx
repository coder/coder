import dayjs from "dayjs";
import { ChevronLeftIcon, ShieldIcon } from "lucide-react";
import { type FC, type FormEvent, useMemo, useState } from "react";
import {
	keepPreviousData,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useSearchParams } from "react-router";
import TextareaAutosize from "react-textarea-autosize";
import { getErrorMessage } from "#/api/errors";
import {
	chatCostSummary,
	chatCostUsers,
	chatDesktopEnabled,
	chatModelConfigs,
	chatSystemPrompt,
	chatTemplateAllowlist,
	chatUserCustomPrompt,
	chatWorkspaceTTL,
	updateChatDesktopEnabled,
	updateChatSystemPrompt,
	updateChatTemplateAllowlist,
	updateChatWorkspaceTTL,
	updateUserChatCustomPrompt,
} from "#/api/queries/chats";
import { templates } from "#/api/queries/templates";
import { user } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import {
	MultiSelectCombobox,
	type Option,
} from "#/components/MultiSelectCombobox/MultiSelectCombobox";
import { PaginationAmount } from "#/components/PaginationWidget/PaginationAmount";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import { SearchField } from "#/components/SearchField/SearchField";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useDebouncedValue } from "#/hooks/debounce";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { formatTokenCount } from "#/utils/analytics";
import { cn } from "#/utils/cn";
import { formatCostMicros } from "#/utils/currency";
import { countInvisibleCharacters } from "#/utils/invisibleUnicode";
import { ChatCostSummaryView } from "./components/ChatCostSummaryView";
import { ChatModelAdminPanel } from "./components/ChatModelAdminPanel/ChatModelAdminPanel";
import {
	DateRangePicker,
	type DateRangeValue,
} from "./components/DateRangePicker/DateRangePicker";
import { DurationField } from "./components/DurationField/DurationField";
import { InsightsContent } from "./components/InsightsContent";
import { LimitsTab } from "./components/LimitsTab";
import { MCPServerAdminPanel } from "./components/MCPServerAdminPanel";
import { SectionHeader } from "./components/SectionHeader";
import { TextPreviewDialog } from "./components/TextPreviewDialog";
import { UserCompactionThresholdSettings } from "./UserCompactionThresholdSettings";

const AdminBadge: FC = () => (
	<TooltipProvider delayDuration={0}>
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="ml-auto inline-flex cursor-default items-center gap-1 rounded bg-surface-tertiary/60 px-2 py-1 text-[11px] leading-none font-medium text-content-secondary">
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

const usageStartDateSearchParam = "startDate";
const usageEndDateSearchParam = "endDate";

const getDefaultUsageDateRange = (now?: dayjs.Dayjs): DateRangeValue => {
	const end = now ?? dayjs();
	return {
		startDate: end.subtract(30, "day").toDate(),
		endDate: end.toDate(),
	};
};

const formatUsageDateRange = (
	value: DateRangeValue,
	options?: {
		endDateIsExclusive?: boolean;
	},
) => {
	// Custom ranges keep the raw API end boundary, which can be midnight on
	// the following day for full-day selections. Show the inclusive day in
	// the drill-in label without changing the query params.
	const displayEndDate =
		options?.endDateIsExclusive &&
		dayjs(value.endDate).isSame(dayjs(value.endDate).startOf("day"))
			? dayjs(value.endDate).subtract(1, "day")
			: dayjs(value.endDate);

	return `${dayjs(value.startDate).format("MMM D")} – ${displayEndDate.format(
		"MMM D, YYYY",
	)}`;
};

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
	const startDateParam =
		searchParams.get(usageStartDateSearchParam)?.trim() ?? "";
	const endDateParam = searchParams.get(usageEndDateSearchParam)?.trim() ?? "";
	const [defaultDateRange] = useState(() => getDefaultUsageDateRange(now));
	let dateRange = defaultDateRange;
	let hasExplicitDateRange = false;

	if (startDateParam && endDateParam) {
		const parsedStartDate = new Date(startDateParam);
		const parsedEndDate = new Date(endDateParam);

		if (
			!Number.isNaN(parsedStartDate.getTime()) &&
			!Number.isNaN(parsedEndDate.getTime()) &&
			parsedStartDate.getTime() <= parsedEndDate.getTime()
		) {
			dateRange = {
				startDate: parsedStartDate,
				endDate: parsedEndDate,
			};
			hasExplicitDateRange = true;
		}
	}

	const dateRangeParams = {
		start_date: dateRange.startDate.toISOString(),
		end_date: dateRange.endDate.toISOString(),
	};
	const { endDate } = dateRange;
	const isExclusiveMidnightEnd =
		hasExplicitDateRange &&
		endDate.getHours() === 0 &&
		endDate.getMinutes() === 0 &&
		endDate.getSeconds() === 0 &&
		endDate.getMilliseconds() === 0;
	const displayDateRange = isExclusiveMidnightEnd
		? {
				startDate: dateRange.startDate,
				endDate: new Date(endDate.getTime() - 1),
			}
		: dateRange;
	const dateRangeLabel = formatUsageDateRange(dateRange, {
		endDateIsExclusive: hasExplicitDateRange,
	});
	const offset = (page - 1) * pageSize;

	const onDateRangeChange = (value: DateRangeValue) => {
		// Reset pagination but preserve user selection and other params.
		setPage(1);
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.set(usageStartDateSearchParam, value.startDate.toISOString());
			next.set(usageEndDateSearchParam, value.endDate.toISOString());
			return next;
		});
	};

	const usersQuery = useQuery({
		...chatCostUsers({
			...dateRangeParams,
			username: debouncedSearch || undefined,
			limit: pageSize,
			offset,
		}),
		placeholderData: keepPreviousData,
	});

	const selectedUserId = searchParams.get("user");
	const selectedUserQuery = useQuery({
		...user(selectedUserId ?? ""),
		enabled: selectedUserId !== null,
	});
	const selectedUser = selectedUserQuery.data ?? null;

	const summaryQuery = useQuery({
		...chatCostSummary(selectedUserId ?? "me", dateRangeParams),
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
				<DateRangePicker
					value={displayDateRange}
					onChange={onDateRangeChange}
					now={now?.toDate()}
				/>
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
						<div>{dateRangeLabel}</div>
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

			{usersQuery.data && (
				<div className="relative">
					{usersQuery.isFetching && !usersQuery.isLoading && (
						<div
							role="status"
							aria-label="Refreshing usage"
							className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
						>
							<Spinner size="lg" loading className="text-content-secondary" />
						</div>
					)}
					{usersQuery.data.users.length === 0 ? (
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
													setSearchParams((prev) => {
														const next = new URLSearchParams(prev);
														next.set("user", u.user_id);
														return next;
													});
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
					)}
				</div>
			)}
		</div>
	);
};

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

	const systemPromptQuery = useQuery({
		...chatSystemPrompt(),
		enabled: canSetSystemPrompt,
	});
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
	const isPromptSaving = isSavingSystemPrompt || isSavingUserPrompt;
	const isSystemPromptDisabled = isPromptSaving || !hasLoadedSystemPrompt;
	const isDesktopSaving = isSavingDesktopEnabled;
	const isTTLSaving = isSavingWorkspaceTTL;
	const isTTLLoading = workspaceTTLQuery.isLoading;

	const handleSaveSystemPrompt = (event: FormEvent) => {
		event.preventDefault();
		if (!hasLoadedSystemPrompt || !isSystemPromptDirty) return;
		saveSystemPrompt(
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
							{userInvisibleCharCount > 0 && (
								<Alert severity="warning">
									This text contains {userInvisibleCharCount} invisible Unicode{" "}
									{userInvisibleCharCount !== 1 ? "characters" : "character"}{" "}
									that could hide content. These will be stripped on save.
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
											This text contains {systemInvisibleCharCount} invisible
											Unicode{" "}
											{systemInvisibleCharCount !== 1
												? "characters"
												: "character"}{" "}
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
										<AdminBadge />
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
				{activeSection === "templates" && canManageChatModelConfigs && (
					<TemplateAllowlistSection />
				)}
			</div>
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

const TemplateAllowlistSection: FC = () => {
	const queryClient = useQueryClient();

	// Fetch all available templates.
	const templatesQuery = useQuery(templates());

	// Fetch current allowlist.
	const allowlistQuery = useQuery(chatTemplateAllowlist());

	const {
		mutate: saveAllowlist,
		isPending: isSaving,
		isError: isSaveError,
	} = useMutation(updateChatTemplateAllowlist(queryClient));

	const [localSelection, setLocalSelection] = useState<Option[] | null>(null);

	// Map all templates to MultiSelectCombobox options.
	const allOptions: Option[] = (templatesQuery.data ?? []).map((t) => ({
		value: t.id,
		label: t.display_name || t.name,
		icon: t.icon,
	}));

	// Build a lookup from template ID to Option for resolving server IDs.
	const optionsByID = new Map(allOptions.map((o) => [o.value, o]));

	// Resolve the server-side allowlist IDs into Option objects.
	const serverSelection: Option[] = (allowlistQuery.data?.template_ids ?? [])
		.map((id) => optionsByID.get(id))
		.filter((o) => o !== undefined);

	const currentSelection = localSelection ?? serverSelection;

	const serverSet = new Set(serverSelection.map((o) => o.value));
	const isDirty =
		localSelection !== null &&
		(localSelection.length !== serverSet.size ||
			localSelection.some((o) => !serverSet.has(o.value)));

	const handleSave = (event: FormEvent) => {
		event.preventDefault();
		if (!isDirty) return;
		saveAllowlist(
			{ template_ids: currentSelection.map((o) => o.value) },
			{ onSuccess: () => setLocalSelection(null) },
		);
	};

	const isLoading = templatesQuery.isLoading || allowlistQuery.isLoading;

	return (
		<div className="space-y-6">
			<SectionHeader
				label="Templates"
				description="Restrict which templates agents can use to create workspaces. When no templates are selected, all templates are available."
				badge={<AdminBadge />}
			/>

			{isLoading && (
				<div
					role="status"
					aria-label="Loading templates"
					className="flex min-h-[120px] items-center justify-center"
				>
					<Spinner size="lg" loading className="text-content-secondary" />
				</div>
			)}

			{!isLoading && (templatesQuery.error || allowlistQuery.error) && (
				<div className="flex min-h-[120px] flex-col items-center justify-center gap-4 text-center">
					<p className="m-0 text-sm text-content-secondary">
						Failed to load template data.
					</p>
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={() => {
							void templatesQuery.refetch();
							void allowlistQuery.refetch();
						}}
					>
						Retry
					</Button>
				</div>
			)}

			{!isLoading && !templatesQuery.error && !allowlistQuery.error && (
				<form
					className="space-y-3"
					onSubmit={(event) => void handleSave(event)}
				>
					<MultiSelectCombobox
						key={serverSelection.map((o) => o.value).join(",")}
						inputProps={{ "aria-label": "Select allowed templates" }}
						options={allOptions}
						defaultOptions={currentSelection}
						value={currentSelection}
						onChange={setLocalSelection}
						placeholder="Select templates..."
						emptyIndicator={
							<p className="text-center text-sm text-content-secondary">
								No templates found.
							</p>
						}
						disabled={isSaving}
						hidePlaceholderWhenSelected
						data-testid="template-allowlist-select"
					/>
					<p
						aria-live="polite"
						role="status"
						className="m-0 text-xs text-content-secondary"
					>
						{currentSelection.length > 0
							? `${currentSelection.length} template${currentSelection.length !== 1 ? "s" : ""} selected`
							: "No templates selected \u2014 all templates are available"}
					</p>

					<div className="flex justify-end">
						<Button size="sm" type="submit" disabled={isSaving || !isDirty}>
							Save
						</Button>
					</div>

					{isSaveError && (
						<p role="alert" className="m-0 text-xs text-content-destructive">
							Failed to save template allowlist.
						</p>
					)}
				</form>
			)}
		</div>
	);
};
