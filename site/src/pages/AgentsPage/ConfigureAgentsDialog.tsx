import { getErrorMessage } from "api/errors";
import {
	chatCostSummary,
	chatCostUsers,
	chatSystemPrompt,
	chatUserCustomPrompt,
	updateChatSystemPrompt,
	updateUserChatCustomPrompt,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { PaginationAmount } from "components/PaginationWidget/PaginationAmount";
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase";
import { SearchField } from "components/SearchField/SearchField";
import { Spinner } from "components/Spinner/Spinner";
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
import type { LucideIcon } from "lucide-react";
import {
	BarChart3Icon,
	BoxesIcon,
	KeyRoundIcon,
	ShieldAlertIcon,
	ShieldIcon,
	UserIcon,
	XIcon,
} from "lucide-react";
import { type FC, type FormEvent, useCallback, useMemo, useState } from "react";
import {
	keepPreviousData,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import TextareaAutosize from "react-textarea-autosize";
import { formatTokenCount } from "utils/analytics";
import { cn } from "utils/cn";
import { formatCostMicros } from "utils/currency";
import { ChatCostSummaryView } from "./ChatCostSummaryView";
import { ChatModelAdminPanel } from "./ChatModelAdminPanel/ChatModelAdminPanel";
import { LimitsTab } from "./LimitsTab";
import { SectionHeader } from "./SectionHeader";

export type ConfigureAgentsSection =
	| "providers"
	| "models"
	| "limits"
	| "behavior"
	| "usage";

type ConfigureAgentsSectionOption = {
	id: ConfigureAgentsSection;
	label: string;
	icon: LucideIcon;
	adminOnly?: boolean;
};

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
	const [selectedUser, setSelectedUser] =
		useState<TypesGen.ChatCostUserRollup | null>(null);
	const [usernameFilter, setUsernameFilter] = useState("");
	const debouncedUsername = useDebouncedValue(usernameFilter, 300);
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
			username: debouncedUsername || undefined,
			limit: pageSize,
			offset,
		}),
		placeholderData: keepPreviousData,
	});
	const summaryQuery = useQuery({
		...chatCostSummary(selectedUser?.user_id ?? "me", {
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
		}),
		enabled: selectedUser !== null,
	});

	const totalCount = usersQuery.data?.count ?? 0;
	const hasPreviousPage = page > 1;
	const hasNextPage = offset + pageSize < totalCount;

	const header = (
		<SectionHeader
			label="Usage"
			description={
				selectedUser
					? "Review deployment chat usage for a specific user."
					: "Review deployment chat usage and drill into individual users."
			}
			badge={<AdminBadge />}
			action={
				selectedUser ? (
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={() => setSelectedUser(null)}
					>
						← Back to all users
					</Button>
				) : (
					<span className="text-xs text-content-secondary">
						{dateRange.rangeLabel}
					</span>
				)
			}
		/>
	);

	if (selectedUser) {
		return (
			<div className="space-y-6">
				{header}
				<div className="flex flex-wrap items-center gap-3 rounded-lg border border-border-default bg-surface-secondary px-4 py-3">
					<AvatarData
						title={selectedUser.name || selectedUser.username}
						subtitle={`@${selectedUser.username}`}
						src={selectedUser.avatar_url}
						imgFallbackText={selectedUser.username}
					/>
					<div className="min-w-0 text-xs text-content-secondary">
						<div>User ID: {selectedUser.user_id}</div>
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
						value={usernameFilter}
						onChange={(value) => {
							setUsernameFilter(value);
							setPage(1);
						}}
						placeholder="Filter by username"
						aria-label="Filter usage by username"
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
											onSelect={setSelectedUser}
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

interface ConfigureAgentsDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	canManageChatModelConfigs: boolean;
	canSetSystemPrompt: boolean;
	initialSection?: ConfigureAgentsSection;
	/** Override the current time for date range calculation. Used for deterministic Storybook snapshots. */
	now?: dayjs.Dayjs;
}

export const ConfigureAgentsDialog: FC<ConfigureAgentsDialogProps> = ({
	open,
	onOpenChange,
	canManageChatModelConfigs,
	canSetSystemPrompt,
	initialSection = "behavior",
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

	const serverPrompt = systemPromptQuery.data?.system_prompt ?? "";
	const [localEdit, setLocalEdit] = useState<string | null>(null);
	const systemPromptDraft = localEdit ?? serverPrompt;

	const serverUserPrompt = userPromptQuery.data?.custom_prompt ?? "";
	const [localUserEdit, setLocalUserEdit] = useState<string | null>(null);
	const userPromptDraft = localUserEdit ?? serverUserPrompt;

	const isSystemPromptDirty = localEdit !== null && localEdit !== serverPrompt;
	const isUserPromptDirty =
		localUserEdit !== null && localUserEdit !== serverUserPrompt;
	const isDisabled = isSavingSystemPrompt || isSavingUserPrompt;

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
	const configureSectionOptions = useMemo<
		readonly ConfigureAgentsSectionOption[]
	>(() => {
		const options: ConfigureAgentsSectionOption[] = [];
		options.push({
			id: "behavior",
			label: "Behavior",
			icon: UserIcon,
		});
		if (canManageChatModelConfigs) {
			options.push({
				id: "providers",
				label: "Providers",
				icon: KeyRoundIcon,
				adminOnly: true,
			});
			options.push({
				id: "models",
				label: "Models",
				icon: BoxesIcon,
				adminOnly: true,
			});
			options.push({
				id: "limits",
				label: "Limits",
				icon: ShieldAlertIcon,
				adminOnly: true,
			});
			options.push({
				id: "usage",
				label: "Usage",
				icon: BarChart3Icon,
				adminOnly: true,
			});
		}
		return options;
	}, [canManageChatModelConfigs]);

	const [userActiveSection, setUserActiveSection] =
		useState<ConfigureAgentsSection>(initialSection);

	const activeSection = configureSectionOptions.some(
		(s) => s.id === userActiveSection,
	)
		? userActiveSection
		: (configureSectionOptions[0]?.id ?? "behavior");

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="grid h-[min(88dvh,720px)] max-w-4xl grid-cols-1 gap-0 overflow-hidden p-0 md:grid-cols-[220px_minmax(0,1fr)]">
				<DialogHeader className="sr-only">
					<DialogTitle>Settings</DialogTitle>
					<DialogDescription>
						Manage your personal preferences and agent configuration.
					</DialogDescription>
				</DialogHeader>

				<nav className="flex flex-row gap-0.5 overflow-x-auto border-b border-border bg-surface-secondary/40 p-2 md:flex-col md:gap-0.5 md:overflow-x-visible md:border-b-0 md:border-r md:p-4">
					<DialogClose asChild>
						<Button
							variant="subtle"
							size="icon-lg"
							className="mb-3 shrink-0 border-none bg-transparent shadow-none hover:bg-surface-tertiary/50"
						>
							<XIcon className="text-content-secondary" />
							<span className="sr-only">Close</span>
						</Button>
					</DialogClose>
					{configureSectionOptions.map((section) => {
						const isActive = section.id === activeSection;
						const SectionIcon = section.icon;
						return (
							<Button
								key={section.id}
								variant="subtle"
								className={cn(
									"h-auto justify-start gap-3 rounded-lg border-none px-3 py-1.5 text-left shadow-none",
									isActive
										? "bg-surface-tertiary/60 text-content-primary hover:bg-surface-tertiary/60"
										: "bg-transparent text-content-secondary hover:bg-surface-tertiary/30 hover:text-content-primary",
								)}
								onClick={() => setUserActiveSection(section.id)}
							>
								<SectionIcon className="h-5 w-5 shrink-0" />
								<span className="flex flex-1 items-center gap-2 text-sm font-medium">
									{section.label}
									{section.adminOnly && (
										<TooltipProvider delayDuration={0}>
											<Tooltip>
												<TooltipTrigger asChild>
													<span className="ml-auto inline-flex">
														<ShieldIcon className="h-3 w-3 shrink-0 opacity-50" />
													</span>
												</TooltipTrigger>
												<TooltipContent side="right">Admin only</TooltipContent>
											</Tooltip>
										</TooltipProvider>
									)}
								</span>
							</Button>
						);
					})}
				</nav>

				<div className="flex min-h-0 flex-1 flex-col overflow-hidden">
					{activeSection === "behavior" && (
						<div className="flex-1 overflow-y-auto px-6 py-5 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
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
								</>
							)}
						</div>
					)}
					{activeSection === "providers" && canManageChatModelConfigs && (
						<div className="flex-1 overflow-y-auto px-6 py-5 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
							<ChatModelAdminPanel
								section="providers"
								sectionLabel="Providers"
								sectionDescription="Connect third-party LLM services like OpenAI, Anthropic, or Google. Each provider supplies models that users can select for their chats."
								sectionBadge={<AdminBadge />}
							/>
						</div>
					)}
					{activeSection === "models" && canManageChatModelConfigs && (
						<div className="flex-1 overflow-y-auto px-6 py-5 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
							<ChatModelAdminPanel
								section="models"
								sectionLabel="Models"
								sectionDescription="Choose which models from your configured providers are available for users to select. You can set a default and adjust context limits."
								sectionBadge={<AdminBadge />}
							/>
						</div>
					)}
					{activeSection === "limits" && canManageChatModelConfigs && (
						<LimitsTab />
					)}
					{activeSection === "usage" && canManageChatModelConfigs && (
						<div className="flex-1 overflow-y-auto px-6 py-5 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
							<UsageContent now={now} />
						</div>
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
};
