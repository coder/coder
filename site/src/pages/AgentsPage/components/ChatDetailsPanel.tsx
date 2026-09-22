import { cn } from "cn";
import { ChevronRightIcon } from "lucide-react";
import {
	type FC,
	type ReactNode,
	type RefObject,
	useId,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";
import { chat, chatCost } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getContextInventory } from "../utils/chatDetails";
import { getChatCostTreeID } from "./ChatConversation/chatHelpers";
import {
	ContextResourceGroups,
	ContextResourceIssues,
	McpResourceList,
} from "./ChatDetailsResources";
import { ChatDetailsUsage } from "./ChatDetailsUsage";
import { ChatSummary } from "./ChatSummary";
import type { AgentContextUsage } from "./ContextUsageIndicator";

type ChatDetailsPanelProps = {
	chatId: string;
	/** Gate reads on tab visibility so hidden Details does not fetch chat or cost. */
	isVisible: boolean;
	usage: AgentContextUsage | null;
	onApplyContext?: () => void;
	isApplyingContext?: boolean;
	applyError?: unknown;
	applySuccess?: boolean;
	workspaceStatus?: string;
	focusRef?: RefObject<HTMLDivElement | null>;
};

export const ChatDetailsPanel: FC<ChatDetailsPanelProps> = ({
	chatId,
	isVisible,
	focusRef,
	...props
}) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });
	const chatData = chatQuery.data;
	const rootChatId = getChatCostTreeID(chatData) ?? chatId;
	const costQuery = useQuery({
		...chatCost(rootChatId),
		enabled: isVisible && showCost && chatData !== undefined,
	});
	let summary: ReactNode = (
		<Skeleton aria-label="Loading summary" className="h-20 w-full" />
	);
	if (chatData)
		summary = (
			<ChatSummary
				summary={chatData.summary}
				isSubagent={Boolean(chatData.parent_chat_id)}
				createdAt={chatData.created_at}
				updatedAt={chatData.updated_at}
				costMicros={costQuery.data?.total_cost_micros}
				unpricedRequestCount={costQuery.data?.unpriced_request_count}
				showCost={showCost}
				isCostLoading={costQuery.isLoading}
				costError={costQuery.isError}
			/>
		);
	else if (chatQuery.isError) summary = <ErrorAlert error={chatQuery.error} />;
	return (
		<div
			ref={focusRef}
			role="region"
			aria-label="Details"
			tabIndex={-1}
			className="h-full min-h-0 overflow-y-auto outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link forced-colors:[&_*]:text-[CanvasText] forced-colors:focus-visible:outline forced-colors:focus-visible:outline-2 forced-colors:focus-visible:outline-[Highlight]"
		>
			<DetailsSections key={chatId} {...props} summary={summary} />
		</div>
	);
};

const DetailsSection: FC<{
	title: string;
	count?: string;
	defaultOpen?: boolean;
	description?: string;
	warning?: boolean;
	compactContent?: ReactNode;
	children: ReactNode;
}> = ({
	title,
	count,
	defaultOpen = false,
	description,
	warning,
	compactContent,
	children,
}) => {
	const [open, setOpen] = useState(defaultOpen);
	const contentId = useId();
	const descriptionId = useId();
	return (
		<Collapsible
			open={open}
			onOpenChange={setOpen}
			className="border-0 border-b border-solid border-border-default last:border-b-0"
		>
			<h3 className="m-0 text-sm font-medium">
				<CollapsibleTrigger asChild>
					<button
						type="button"
						aria-controls={contentId}
						aria-describedby={description ? descriptionId : undefined}
						className="flex min-h-12 w-full flex-wrap items-center gap-2 border-0 bg-transparent px-4 py-3 text-left text-content-primary hover:bg-surface-secondary focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-content-link"
					>
						<ChevronRightIcon
							aria-hidden="true"
							className={cn("size-4 shrink-0", open && "rotate-90")}
						/>
						<span className="min-w-0 flex-1 basis-24 wrap-anywhere">
							{title}
						</span>{" "}
						{count && (
							<span
								className={cn(
									"min-w-0 text-xs font-normal wrap-anywhere",
									warning ? "text-highlight-orange" : "text-content-secondary",
								)}
							>
								{count}
							</span>
						)}{" "}
						{!open && compactContent}
					</button>
				</CollapsibleTrigger>
			</h3>
			{description && (
				<p
					id={descriptionId}
					className={cn(
						"m-0 px-4 pb-3 text-xs",
						warning ? "text-highlight-orange" : "text-content-secondary",
					)}
				>
					{description}
				</p>
			)}
			<CollapsibleContent id={contentId} className="px-4 pb-4 text-sm">
				<div className="flex min-w-0 flex-col gap-4">{children}</div>
			</CollapsibleContent>
		</Collapsible>
	);
};

const DetailsSections: FC<
	Omit<ChatDetailsPanelProps, "chatId" | "isVisible" | "focusRef"> & {
		summary: ReactNode;
	}
> = ({
	usage,
	summary,
	workspaceStatus,
	onApplyContext,
	isApplyingContext = false,
	applyError,
	applySuccess,
}) => {
	const inventory = getContextInventory(usage?.context);
	const context = usage?.context;
	const contextTarget = useRef<HTMLDivElement>(null);
	const applyHadFocus = useRef(false);
	const showApply = Boolean(
		onApplyContext &&
			(context?.dirty || context?.error || applyError || isApplyingContext),
	);
	useLayoutEffect(() => {
		// Removing a focused action must not drop keyboard users onto the body.
		if (
			!showApply &&
			applyHadFocus.current &&
			document.activeElement === document.body
		) {
			contextTarget.current?.focus();
			applyHadFocus.current = false;
		}
	}, [showApply]);
	const staleWorkspace =
		workspaceStatus && !["running", "connected"].includes(workspaceStatus);
	return (
		<>
			<DetailsSection title="Summary" defaultOpen>
				{summary}
			</DetailsSection>
			<div
				ref={contextTarget}
				role="group"
				aria-label="Workspace context"
				tabIndex={-1}
				className="outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link forced-colors:focus-visible:outline forced-colors:focus-visible:outline-2 forced-colors:focus-visible:outline-[Highlight]"
			>
				{Boolean(
					context?.dirty || context?.error || applyError || showApply,
				) && (
					<div className="flex flex-col gap-2 border-0 border-b border-solid border-border-default p-4 text-xs">
						{context?.dirty && (
							<p className="m-0 text-highlight-orange">
								New workspace context is available. This chat is using the
								snapshot shown below.
							</p>
						)}
						{context?.error && (
							<p className="m-0 text-highlight-red wrap-anywhere">
								Context could not be loaded: {context.error}
							</p>
						)}
						{Boolean(applyError) && <ErrorAlert error={applyError} />}
						{showApply && (
							<>
								<p className="m-0 text-content-secondary">
									Updates the instructions and skills used by subsequent turns.
								</p>
								<Button
									className="h-auto min-h-8 self-start whitespace-normal text-left forced-colors:focus-visible:outline forced-colors:focus-visible:outline-2 forced-colors:focus-visible:outline-[Highlight]"
									size="sm"
									aria-disabled={isApplyingContext}
									aria-busy={isApplyingContext}
									onFocus={() => {
										applyHadFocus.current = true;
									}}
									onBlur={() => {
										applyHadFocus.current = false;
									}}
									onClick={() => {
										if (!isApplyingContext) onApplyContext?.();
									}}
								>
									<span aria-hidden="true">
										<Spinner size="sm" loading={isApplyingContext} />
									</span>
									{isApplyingContext
										? "Applying latest context..."
										: applyError || context?.error
											? "Retry applying latest context"
											: "Apply latest context"}
								</Button>
							</>
						)}
					</div>
				)}
				<div
					role="status"
					aria-live="polite"
					className={cn(
						"text-xs text-content-secondary",
						(applySuccess || isApplyingContext) && "px-4 py-2",
					)}
				>
					{isApplyingContext
						? "Applying latest context."
						: applySuccess
							? "Latest context applied."
							: ""}
				</div>
				<DetailsSection
					title="Context"
					count={
						inventory.known
							? `${inventory.files.length} ${inventory.files.length === 1 ? "file" : "files"}`
							: "Unknown"
					}
					compactContent={<ChatDetailsUsage usage={usage} compact />}
				>
					<ChatDetailsUsage usage={usage} />
					{inventory.files.length === 0 && (
						<p className="m-0 text-content-secondary">
							{inventory.known
								? "No instruction files."
								: "Instruction files unavailable."}
						</p>
					)}
					<ContextResourceGroups items={inventory.files} kind="file" />
					<ContextResourceIssues
						items={inventory.issues.filter(
							({ resource }) => resource.kind === "instruction_file",
						)}
					/>
				</DetailsSection>
				<DetailsSection
					title="Skills"
					count={
						inventory.known
							? `${inventory.skills.length} ${inventory.skills.length === 1 ? "skill" : "skills"}`
							: "Unknown"
					}
				>
					{inventory.skills.length === 0 && (
						<p className="m-0 text-content-secondary">
							{inventory.known ? "No available skills." : "Skills unavailable."}
						</p>
					)}
					<ContextResourceGroups items={inventory.skills} kind="skill" />
					<ContextResourceIssues
						items={inventory.issues.filter(
							({ resource }) => resource.kind === "skill",
						)}
					/>
				</DetailsSection>
				<DetailsSection
					title="MCP servers"
					count={
						inventory.known
							? inventory.connectedServers +
								" of " +
								inventory.servers.length +
								" connected"
							: "Unknown"
					}
					warning={
						inventory.connectedServers < inventory.servers.length ||
						Boolean(staleWorkspace)
					}
					description={
						"Status from latest workspace sync" +
						(staleWorkspace
							? `. Workspace ${workspaceStatus}; status may be stale.`
							: ".")
					}
				>
					{inventory.servers.length === 0 && (
						<p className="m-0 text-content-secondary">
							{inventory.known
								? "No MCP servers."
								: "MCP server inventory unavailable."}
						</p>
					)}
					<McpResourceList
						configs={inventory.configs}
						servers={inventory.servers}
					/>
					<ContextResourceIssues
						items={inventory.issues.filter(
							({ resource }) => resource.kind === "mcp_config",
						)}
					/>
				</DetailsSection>
			</div>
		</>
	);
};
