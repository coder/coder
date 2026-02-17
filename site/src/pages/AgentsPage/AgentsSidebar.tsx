import { type FC, useMemo, useState } from "react";
import type { Chat, ChatDiffStatus, ChatStatus } from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { shortRelativeTime } from "utils/time";
import { cn } from "utils/cn";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import {
	ArchiveIcon,
	AlertCircleIcon,
	CheckCircle2Icon,
	EllipsisIcon,
	EditIcon,
	Loader2Icon,
	PauseCircleIcon,
	PlusIcon,
	SearchIcon,
} from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { NavLink } from "react-router";

interface AgentsSidebarProps {
	chats: readonly Chat[];
	chatErrorReasons: Record<string, string>;
	modelOptions: readonly ModelSelectorOption[];
	selectedChatId?: string;
	onSelect: (chatId: string) => void;
	onArchiveAgent: (chatId: string) => void;
	onNewAgent: () => void;
	isCreating: boolean;
	isArchiving?: boolean;
	archivingChatId?: string | null;
}

const statusConfig = {
	waiting: { icon: CheckCircle2Icon, className: "text-content-success" },
	pending: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	running: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	paused: { icon: PauseCircleIcon, className: "text-content-warning" },
	error: { icon: AlertCircleIcon, className: "text-content-destructive" },
	completed: { icon: CheckCircle2Icon, className: "text-content-success" },
} as const;

const getStatusConfig = (status: ChatStatus) => {
	return statusConfig[status] ?? statusConfig.completed;
};

const getModelDisplayName = (
	modelConfig: Chat["model_config"] | undefined,
	modelOptions: readonly ModelSelectorOption[],
) => {
	if (!modelConfig || typeof modelConfig !== "object") {
		return "Default model";
	}

	const model = (modelConfig as { model?: string }).model;
	if (!model) {
		return "Default model";
	}

	// Try to find a matching option with a display name.
	const match = modelOptions.find(
		(opt) => opt.id === model || opt.model === model,
	);
	if (match?.displayName) {
		return match.displayName;
	}

	// Fall back to stripping the provider prefix.
	const parts = model.split(":");
	if (parts.length === 2) {
		return parts[1];
	}

	return model;
};

type ChatWithDiffStatus = Chat & {
	readonly diff_status?: ChatDiffStatus;
};

const getChatDiffStatus = (chat: Chat): ChatDiffStatus | undefined => {
	return (chat as ChatWithDiffStatus).diff_status;
};

export const AgentsSidebar: FC<AgentsSidebarProps> = (props) => {
	const {
		chats,
		chatErrorReasons,
		modelOptions,
		onArchiveAgent,
		onNewAgent,
		isCreating,
		isArchiving = false,
		archivingChatId = null,
	} = props;
	const [search, setSearch] = useState("");

	const filteredChats = useMemo(
		() =>
			chats.filter((chat) =>
				chat.title.toLowerCase().includes(search.toLowerCase()),
			),
		[chats, search],
	);

	return (
		<div className="flex h-full w-full min-h-0 flex-col border-0 border-r border-solid">
			<div className="border-b border-border-default px-3 pb-3 pt-4 md:px-3.5">
				<div className="flex flex-col gap-2.5">
					<div className="relative">
						<label className="sr-only" htmlFor="agents-sidebar-search">
							Search agents...
						</label>
						<SearchIcon className="pointer-events-none absolute left-3 top-1/2 size-icon-xs -translate-y-1/2 text-content-secondary" />
						<Input
							id="agents-sidebar-search"
							type="search"
							placeholder="Search agents..."
							value={search}
							onChange={(event) => setSearch(event.target.value)}
							className="h-9 rounded-lg border-border-default bg-surface-primary pl-8 text-sm shadow-none"
						/>
					</div>
					<Button
						size="sm"
						variant="outline"
						onClick={onNewAgent}
						disabled={isCreating}
						className="w-full justify-center rounded-lg hover:bg-surface-tertiary text-sm py-4 text-content-secondary"
					>
						New Agent
					</Button>
				</div>
			</div>

			<ScrollArea className="flex-1 [&_[data-radix-scroll-area-viewport]>div]:!block" scrollBarClassName="w-1.5">
				<div className="flex flex-col gap-2 px-2 py-3 md:px-2">
					<div className="ml-2.5 flex items-center justify-between text-xs font-medium text-content-secondary">
						<span>This Week</span>
					</div>

					<div className="flex flex-col gap-0.5">
							{filteredChats.map((chat) => {
								const config = getStatusConfig(chat.status);
								const StatusIcon = config.icon;
								const modelName = getModelDisplayName(chat.model_config, modelOptions);
								const errorReason =
									chat.status === "error" ? chatErrorReasons[chat.id] : undefined;
								const subtitle = errorReason || modelName;
								const diffStatus = getChatDiffStatus(chat);
								const hasLinkedDiffStatus = Boolean(diffStatus?.pull_request_url);
								const changedFiles = diffStatus?.changed_files ?? 0;
								const additions = diffStatus?.additions ?? 0;
								const deletions = diffStatus?.deletions ?? 0;
								const hasLineStats = additions > 0 || deletions > 0;
								const filesChangedLabel = `${changedFiles} ${
									changedFiles === 1 ? "file" : "files"
								}`;
								const isArchivingThisChat =
									isArchiving && archivingChatId === chat.id;

								return (
									<div
										key={chat.id}
										className={cn(
											"group relative flex min-w-0 items-start gap-2 rounded-md pr-1 text-content-secondary",
											"transition-none hover:bg-surface-tertiary hover:text-content-primary has-[[data-state=open]]:bg-surface-tertiary",
											"has-[[aria-current=page]]:bg-surface-quaternary/25 has-[[aria-current=page]]:text-content-primary has-[[aria-current=page]]:hover:bg-surface-quaternary/50",
										)}
									>
										<NavLink
											to={`/agents/${chat.id}`}
											className="flex min-h-0 min-w-0 flex-1 items-start gap-2 rounded-[inherit] px-2 py-2 text-inherit no-underline"
										>
											{({ isActive }) => (
												<>
													<div className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md">
														<StatusIcon
															className={cn(
																"h-3.5 w-3.5 shrink-0",
																config.className,
															)}
														/>
													</div>
													<div className="min-w-0 flex-1 overflow-hidden space-y-1 text-left">
														<div className="flex min-w-0 items-center gap-2 overflow-hidden">
															<span
																className={cn(
																	"block flex-1 truncate text-xs",
																	isActive && "font-medium",
																)}
															>
																{chat.title}
															</span>
														</div>
														<div className="flex min-w-0 items-center gap-1.5">
															<div
																className={cn(
																	"min-w-0 overflow-hidden text-xs leading-4",
																	errorReason
																		? "line-clamp-1 whitespace-normal text-content-destructive [overflow-wrap:anywhere]"
																		: "truncate text-content-secondary",
																)}
																title={subtitle}
															>
																{subtitle}
															</div>
															{hasLinkedDiffStatus && hasLineStats && (
																<span
																	className="inline-flex shrink-0 items-center gap-0.5 text-[10px] font-semibold leading-none tabular-nums"
																	title={`${filesChangedLabel}, +${additions} -${deletions}`}
																>
																	<span className="text-content-success">
																		+{additions}
																	</span>
																	<span className="text-content-destructive">
																		-{deletions}
																	</span>
																</span>
															)}
														</div>
													</div>
												</>
											)}
										</NavLink>
										<div className="relative mt-1 h-6 w-7 shrink-0 mr-1 text-right">
											<span className="absolute inset-0 flex items-center justify-end text-xs text-content-secondary tabular-nums transition-opacity group-hover:opacity-0">
												{shortRelativeTime(chat.updated_at)}
											</span>
											<DropdownMenu>
												<DropdownMenuTrigger asChild>
													<Button
														size="icon"
														variant="subtle"
														className={cn(
															"absolute inset-0 h-6 w-7 justify-end rounded-none px-0 text-content-secondary opacity-0 transition-opacity hover:text-content-primary group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100",
															isArchivingThisChat && "opacity-100",
														)}
														aria-label={`Open actions for ${chat.title}`}
													>
														{isArchivingThisChat ? (
															<Loader2Icon className="h-3.5 w-3.5 animate-spin" />
														) : (
															<EllipsisIcon className="h-3.5 w-3.5" />
														)}
													</Button>
												</DropdownMenuTrigger>
												<DropdownMenuContent align="end">
													<DropdownMenuItem
														onSelect={(event) => event.preventDefault()}
													>
														Mark as unread
													</DropdownMenuItem>
													<DropdownMenuItem
														className="text-content-destructive focus:text-content-destructive"
														disabled={isArchiving}
														onSelect={() => onArchiveAgent(chat.id)}
													>
														<ArchiveIcon className="h-3.5 w-3.5" />
														Archive agent
													</DropdownMenuItem>
												</DropdownMenuContent>
											</DropdownMenu>
										</div>
									</div>
								);
							})}

							{filteredChats.length === 0 && (
								<div className="rounded-lg border border-dashed border-border-default bg-surface-primary p-4 text-center text-xs text-content-secondary">
									{search ? "No matching agents" : "No agents yet"}
								</div>
							)}
					</div>
				</div>
			</ScrollArea>
		</div>
	);
};
