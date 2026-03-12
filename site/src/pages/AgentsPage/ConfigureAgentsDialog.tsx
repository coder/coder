import {
	chatCostSummary,
	chatCostUsers,
	chatSystemPrompt,
	chatUserCustomPrompt,
	updateChatSystemPrompt,
	updateUserChatCustomPrompt,
} from "api/queries/chats";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import dayjs from "dayjs";
import type { LucideIcon } from "lucide-react";
import {
	BarChart3Icon,
	BoxesIcon,
	KeyRoundIcon,
	Loader2Icon,
	ShieldIcon,
	TriangleAlertIcon,
	UserIcon,
	XIcon,
} from "lucide-react";
import {
	type FC,
	type FormEvent,
	useCallback,
	useEffect,
	useMemo,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import TextareaAutosize from "react-textarea-autosize";
import { formatCostMicros, formatTokenCount } from "utils/analytics";
import { cn } from "utils/cn";
import { ChatModelAdminPanel } from "./ChatModelAdminPanel/ChatModelAdminPanel";
import { SectionHeader } from "./SectionHeader";

type ConfigureAgentsSection = "providers" | "models" | "behavior" | "usage";

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

const truncateUserID = (userID: string): string => {
	if (userID.length <= 16) {
		return userID;
	}
	return `${userID.slice(0, 8)}…${userID.slice(-4)}`;
};

const UsageContent: FC = () => {
	const [selectedUserId, setSelectedUserId] = useState<string | null>(null);
	const dateRange = useMemo(() => {
		const end = dayjs();
		const start = end.subtract(30, "day");
		return {
			startDate: start.format("YYYY-MM-DD"),
			endDate: end.format("YYYY-MM-DD"),
			rangeLabel: `${start.format("MMM D")} – ${end.format("MMM D, YYYY")}`,
		};
	}, []);

	const usersQuery = useQuery(
		chatCostUsers({
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
		}),
	);
	const summaryQuery = useQuery({
		...chatCostSummary({
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
			user_id: selectedUserId ?? undefined,
		}),
		enabled: selectedUserId !== null,
	});

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
				selectedUserId ? (
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={() => setSelectedUserId(null)}
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

	if (selectedUserId) {
		return (
			<div className="space-y-6">
				{header}
				<div className="space-y-1">
					<p className="m-0 text-xs text-content-secondary">
						{dateRange.rangeLabel}
					</p>
					<p className="m-0 break-all rounded-lg border border-border bg-surface-secondary px-3 py-2 font-mono text-xs text-content-secondary">
						User ID: {selectedUserId}
					</p>
				</div>

				{summaryQuery.isLoading && (
					<div
						role="status"
						aria-label="Loading usage details"
						className="flex min-h-[240px] items-center justify-center"
					>
						<Loader2Icon className="h-8 w-8 animate-spin text-content-secondary" />
					</div>
				)}

				{summaryQuery.isError && (
					<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
						<p className="m-0 text-sm text-content-secondary">
							{summaryQuery.error instanceof Error
								? summaryQuery.error.message
								: "Failed to load usage details."}
						</p>
						<Button
							variant="outline"
							size="sm"
							type="button"
							onClick={() => void summaryQuery.refetch()}
						>
							Retry
						</Button>
					</div>
				)}

				{summaryQuery.data && (
					<>
						<div className="grid grid-cols-2 gap-4 md:grid-cols-4">
							<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
								<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
									Total Cost
								</p>
								<p className="mt-1 text-2xl font-semibold text-content-primary">
									{formatCostMicros(summaryQuery.data.total_cost_micros)}
								</p>
							</div>
							<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
								<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
									Input Tokens
								</p>
								<p className="mt-1 text-2xl font-semibold text-content-primary">
									{formatTokenCount(summaryQuery.data.total_input_tokens)}
								</p>
							</div>
							<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
								<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
									Output Tokens
								</p>
								<p className="mt-1 text-2xl font-semibold text-content-primary">
									{formatTokenCount(summaryQuery.data.total_output_tokens)}
								</p>
							</div>
							<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
								<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
									Messages
								</p>
								<p className="mt-1 text-2xl font-semibold text-content-primary">
									{(
										summaryQuery.data.priced_message_count +
										summaryQuery.data.unpriced_message_count
									).toLocaleString()}
								</p>
							</div>
						</div>

						{summaryQuery.data.unpriced_message_count > 0 && (
							<div className="flex items-start gap-3 rounded-lg border border-border-warning bg-surface-warning p-4 text-sm text-content-primary">
								<TriangleAlertIcon className="h-5 w-5 shrink-0 text-content-warning" />
								<span>
									{summaryQuery.data.unpriced_message_count} message
									{summaryQuery.data.unpriced_message_count === 1 ? "" : "s"}{" "}
									could not be priced because model pricing data was
									unavailable.
								</span>
							</div>
						)}

						{summaryQuery.data.by_model.length === 0 &&
						summaryQuery.data.by_chat.length === 0 ? (
							<p className="py-12 text-center text-content-secondary">
								No usage data for this user in the selected period.
							</p>
						) : (
							<>
								<div className="overflow-x-auto rounded-lg border border-border-default">
									<table className="w-full text-sm">
										<thead>
											<tr className="text-left text-xs font-medium uppercase tracking-wide text-content-secondary">
												<th className="px-4 py-3">Model</th>
												<th className="px-4 py-3">Provider</th>
												<th className="px-4 py-3 text-right">Cost</th>
												<th className="px-4 py-3 text-right">Messages</th>
												<th className="px-4 py-3 text-right">Input</th>
												<th className="px-4 py-3 text-right">Output</th>
											</tr>
										</thead>
										<tbody>
											{summaryQuery.data.by_model.map((model) => (
												<tr
													key={model.model_config_id}
													className="border-t border-border-default"
												>
													<td className="px-4 py-3">{model.display_name}</td>
													<td className="px-4 py-3 text-content-secondary">
														{model.provider}
													</td>
													<td className="px-4 py-3 text-right">
														{formatCostMicros(model.total_cost_micros)}
													</td>
													<td className="px-4 py-3 text-right">
														{model.message_count.toLocaleString()}
													</td>
													<td className="px-4 py-3 text-right">
														{formatTokenCount(model.total_input_tokens)}
													</td>
													<td className="px-4 py-3 text-right">
														{formatTokenCount(model.total_output_tokens)}
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>

								<div className="overflow-x-auto rounded-lg border border-border-default">
									<table className="w-full text-sm">
										<thead>
											<tr className="text-left text-xs font-medium uppercase tracking-wide text-content-secondary">
												<th className="px-4 py-3">Chat</th>
												<th className="px-4 py-3 text-right">Cost</th>
												<th className="px-4 py-3 text-right">Messages</th>
												<th className="px-4 py-3 text-right">Input</th>
												<th className="px-4 py-3 text-right">Output</th>
											</tr>
										</thead>
										<tbody>
											{summaryQuery.data.by_chat.map((chat) => (
												<tr
													key={chat.root_chat_id}
													className="border-t border-border-default"
												>
													<td className="px-4 py-3">{chat.chat_title}</td>
													<td className="px-4 py-3 text-right">
														{formatCostMicros(chat.total_cost_micros)}
													</td>
													<td className="px-4 py-3 text-right">
														{chat.message_count.toLocaleString()}
													</td>
													<td className="px-4 py-3 text-right">
														{formatTokenCount(chat.total_input_tokens)}
													</td>
													<td className="px-4 py-3 text-right">
														{formatTokenCount(chat.total_output_tokens)}
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
							</>
						)}
					</>
				)}
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{header}
			{usersQuery.isLoading && (
				<div
					role="status"
					aria-label="Loading usage"
					className="flex min-h-[240px] items-center justify-center"
				>
					<Loader2Icon className="h-8 w-8 animate-spin text-content-secondary" />
				</div>
			)}

			{usersQuery.isError && (
				<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
					<p className="m-0 text-sm text-content-secondary">
						{usersQuery.error instanceof Error
							? usersQuery.error.message
							: "Failed to load usage data."}
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
					<div className="overflow-x-auto rounded-lg border border-border-default">
						<table className="w-full text-sm">
							<thead>
								<tr className="text-left text-xs font-medium uppercase tracking-wide text-content-secondary">
									<th className="px-4 py-3">User ID</th>
									<th className="px-4 py-3 text-right">Total Cost</th>
									<th className="px-4 py-3 text-right">Messages</th>
									<th className="px-4 py-3 text-right">Chats</th>
									<th className="px-4 py-3 text-right">Input Tokens</th>
									<th className="px-4 py-3 text-right">Output Tokens</th>
								</tr>
							</thead>
							<tbody>
								{usersQuery.data.users.map((user) => (
									<tr
										key={user.user_id}
										className="cursor-pointer border-t border-border-default transition-colors hover:bg-surface-secondary/50"
										onClick={() => setSelectedUserId(user.user_id)}
										onKeyDown={(event) => {
											if (event.key === "Enter" || event.key === " ") {
												event.preventDefault();
												setSelectedUserId(user.user_id);
											}
										}}
										role="button"
										tabIndex={0}
									>
										<td className="px-4 py-3 font-mono text-xs text-content-primary">
											<span title={user.user_id}>
												{truncateUserID(user.user_id)}
											</span>
										</td>
										<td className="px-4 py-3 text-right">
											{formatCostMicros(user.total_cost_micros)}
										</td>
										<td className="px-4 py-3 text-right">
											{user.message_count.toLocaleString()}
										</td>
										<td className="px-4 py-3 text-right">
											{user.chat_count.toLocaleString()}
										</td>
										<td className="px-4 py-3 text-right">
											{formatTokenCount(user.total_input_tokens)}
										</td>
										<td className="px-4 py-3 text-right">
											{formatTokenCount(user.total_output_tokens)}
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
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
}

export const ConfigureAgentsDialog: FC<ConfigureAgentsDialogProps> = ({
	open,
	onOpenChange,
	canManageChatModelConfigs,
	canSetSystemPrompt,
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
				id: "usage",
				label: "Usage",
				icon: BarChart3Icon,
				adminOnly: true,
			});
		}
		return options;
	}, [canManageChatModelConfigs]);

	const [userActiveSection, setUserActiveSection] =
		useState<ConfigureAgentsSection>("behavior");

	const activeSection = configureSectionOptions.some(
		(s) => s.id === userActiveSection,
	)
		? userActiveSection
		: (configureSectionOptions[0]?.id ?? "behavior");

	useEffect(() => {
		if (open) {
			setUserActiveSection("behavior");
		}
	}, [open]);

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
								<span className="flex items-center gap-2 text-sm font-medium">
									{section.label}
									{section.adminOnly && (
										<TooltipProvider delayDuration={0}>
											<Tooltip>
												<TooltipTrigger asChild>
													<span className="inline-flex">
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

				<div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-5 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
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
									Personal Instructions{" "}
								</h3>
								<p className="!mt-0.5 m-0 text-xs text-content-secondary">
									Applied to all your chats. Only visible to you.
								</p>{" "}
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
									</Button>{" "}
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
										</p>{" "}
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
											</Button>{" "}
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
					{activeSection === "usage" && canManageChatModelConfigs && (
						<UsageContent />
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
};
