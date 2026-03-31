import {
	ChevronDownIcon,
	ChevronRightIcon,
	InfoIcon,
	LoaderIcon,
} from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import type {
	AIBridgeAgenticAction,
	AIBridgeThread,
	MinimalUser,
} from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";
import { StatusIndicatorDot } from "#/components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";
import { JsonPrettyPrinter } from "../../JsonPrettyPrinter";
import { TokenBadges } from "../../TokenBadges";
import { AgenticLoopTable } from "./AgenticLoopTable";
import { PromptTable } from "./PromptTable";
import { ToolCallTable } from "./ToolCallTable";

const EXPANDABLE_COLLAPSE_HEIGHT = 50;

interface ExpandableTextProps {
	text: string;
	className?: string;
}

const ExpandableText: FC<ExpandableTextProps> = ({ text, className }) => {
	const contentRef = useRef<HTMLParagraphElement>(null);
	const [isExpandable, setIsExpandable] = useState(false);
	const [isExpanded, setIsExpanded] = useState(false);

	useEffect(() => {
		const el = contentRef.current;
		if (!el) return;
		setIsExpandable(el.scrollHeight > EXPANDABLE_COLLAPSE_HEIGHT);
	}, []);

	return (
		<div className="relative">
			<p
				ref={contentRef}
				style={
					isExpandable && !isExpanded
						? {
								maxHeight: EXPANDABLE_COLLAPSE_HEIGHT,
							}
						: undefined
				}
				className={cn(className, "overflow-hidden", isExpanded && "pb-9")}
			>
				{text}
			</p>
			{isExpandable && (
				<div
					className={cn(
						"flex justify-end mt-1 absolute bottom-0 right-0 left-0",
						!isExpanded &&
							"bg-gradient-to-t from-surface-primary to-transparent",
					)}
				>
					<Button
						size="sm"
						variant="outline"
						className="bg-surface-primary shadow-sm"
						onClick={() => setIsExpanded((v) => !v)}
					>
						{isExpanded ? "Collapse" : "Show more"}
					</Button>
				</div>
			)}
		</div>
	);
};

interface CollapseButtonProps {
	isOpen: boolean;
	onClick: () => void;
	children: React.ReactNode;
	className?: string;
}

const CollapseButton: FC<CollapseButtonProps> = ({
	isOpen,
	onClick,
	children,
}) => (
	<Button
		type="button"
		variant="subtle"
		onClick={onClick}
		className="border-none bg-transparent text-content-secondary flex items-center"
		size="sm"
	>
		{isOpen ? (
			<ChevronDownIcon className="size-3.5 flex-shrink-0" />
		) : (
			<ChevronRightIcon className="size-3.5 flex-shrink-0" />
		)}
		{children}
	</Button>
);

// Wraps content with a visual left-bracket connector: two rounded corner lines
// that flank the content row, creating an indented visual grouping.
interface BracketConnectorProps {
	children: React.ReactNode;
	contentClassName?: string;
	firstRowHeight?: "2rem" | "60px";
	hideBottomLine?: boolean;
}

const BracketConnector: FC<BracketConnectorProps> = ({
	children,
	contentClassName,
	firstRowHeight = "2rem",
	hideBottomLine = false,
}) => (
	<div
		className={cn(
			"grid grid-cols-[1rem_1rem_1fr]",
			firstRowHeight === "60px"
				? "grid-rows-[60px_auto]"
				: "grid-rows-[2rem_auto]",
		)}
	>
		<div className="row-start-1 col-start-2 border-0 border-b border-l border-solid rounded-bl-lg">
			{/* top rounded line */}
		</div>
		{!hideBottomLine && (
			<div className="row-start-2 col-start-2 border-0 border-t border-l border-solid rounded-tl-lg -mt-px">
				{/* bottom rounded line */}
			</div>
		)}
		<div className={cn("row-start-1 col-start-3 row-span-2", contentClassName)}>
			{children}
		</div>
	</div>
);

interface ThinkingBlockProps {
	text: string;
}

const ThinkingBlock: FC<ThinkingBlockProps> = ({ text }) => (
	<BracketConnector contentClassName="mt-5 pl-2 pr-4 text-sm text-content-secondary">
		<div className="flex items-center">
			<LoaderIcon className="size-icon-xs text-content-secondary" />
			<span className="font-mono ml-2 text-xs">Thinking...</span>
		</div>
		<ExpandableText text={text} className="text-sm text-pretty m-0" />
	</BracketConnector>
);

interface ToolCallBlockProps {
	tool: string;
	serverURL: string;
	input: string;
	inputTokens: number;
	outputTokens: number;
	timestamp: Date;
	tokenUsageMetadata?: Record<string, unknown>;
	expandedByDefault?: boolean;
}

const ToolCallBlock: FC<ToolCallBlockProps> = ({
	tool,
	serverURL,
	input,
	inputTokens,
	outputTokens,
	timestamp,
	tokenUsageMetadata,
	expandedByDefault = false,
}) => {
	const [isOpen, setIsOpen] = useState(expandedByDefault);

	return (
		<BracketConnector contentClassName="mt-2 mr-4 border border-solid rounded-md overflow-x-auto">
			<div className="flex items-center">
				<CollapseButton isOpen={isOpen} onClick={() => setIsOpen(!isOpen)}>
					<span className="text-sm">Tool call</span>
					<Badge size="xs" className="font-mono ml-1">
						{tool}
					</Badge>
				</CollapseButton>
			</div>
			{isOpen && (
				<>
					<ToolCallTable
						className="mt-2 ml-5 mr-4 lg:w-1/2 overflow-x-auto"
						timestamp={timestamp}
						serverURL={serverURL}
						inputTokens={inputTokens}
						outputTokens={outputTokens}
						tokenUsageMetadata={tokenUsageMetadata}
					/>
					<pre className="bg-surface-secondary rounded-md m-4 p-4 text-sm font-mono text-content-primary overflow-x-auto m-0">
						{tool} <JsonPrettyPrinter input={input} />
					</pre>
				</>
			)}
		</BracketConnector>
	);
};

interface AgenticLoopCompletedBlockProps {
	inputTokens: number;
	outputTokens: number;
	expandedByDefault?: boolean;
}

const AgenticLoopCompletedBlock: FC<AgenticLoopCompletedBlockProps> = ({
	inputTokens,
	outputTokens,
	expandedByDefault = false,
}) => {
	const [isOpen, setIsOpen] = useState(expandedByDefault);

	return (
		<BracketConnector
			contentClassName="mt-3 border border-solid rounded-md mb-4 mr-4"
			hideBottomLine
		>
			<div className="flex items-center">
				<CollapseButton isOpen={isOpen} onClick={() => setIsOpen(!isOpen)}>
					<span className="text-sm">Agentic loop completed</span>
				</CollapseButton>
			</div>
			{isOpen && (
				<div className="mb-4 ml-3 mr-4 flex flex-col gap-2 lg:w-1/2 text-sm text-content-secondary">
					<div className="flex items-center justify-between">
						<span className="font-medium">In / out tokens</span>
						<TokenBadges
							inputTokens={inputTokens}
							outputTokens={outputTokens}
						/>
					</div>
				</div>
			)}
		</BracketConnector>
	);
};

interface AgenticActionItemProps {
	action: AIBridgeAgenticAction;
}

const AgenticActionItem: FC<AgenticActionItemProps> = ({ action }) => {
	return (
		<>
			{/* thinking blocks */}
			{action.thinking.map((t) => (
				<ThinkingBlock key={t.text} text={t.text} />
			))}

			{/* tool call blocks */}
			{action.tool_calls.map((tool_call) => (
				<ToolCallBlock
					key={tool_call.id}
					tool={tool_call.tool}
					serverURL={tool_call.server_url}
					input={tool_call.input}
					inputTokens={action.token_usage.input_tokens}
					outputTokens={action.token_usage.output_tokens}
					tokenUsageMetadata={tool_call.metadata}
					timestamp={new Date(tool_call.created_at)}
				/>
			))}
		</>
	);
};

interface ThreadItemProps {
	thread: AIBridgeThread;
	initiator: MinimalUser;
}

const ThreadItem: FC<ThreadItemProps> = ({ thread, initiator }) => {
	const [agenticLoopOpen, setAgenticLoopOpen] = useState(false);

	const durationInMs =
		new Date(thread.ended_at ?? Date.now()).getTime() -
		new Date(thread.started_at).getTime();

	const toolCalls = thread.agentic_actions?.reduce(
		(count, action) => count + action.tool_calls.length,
		0,
	);

	return (
		<>
			<div className="border border-solid rounded-md flex flex-col lg:flex-row gap-6 p-2">
				{/* left column: avatar and username */}
				<div className="flex flex-row items-items-start gap-1">
					<Avatar
						src={initiator.avatar_url}
						fallback={initiator.name ?? initiator.username}
						size="sm"
						className="flex-shrink-0"
					/>
					<span className="text-sm text-content-secondary font-normal py-1">
						{initiator.username}
					</span>
				</div>

				{/* center column: prompt */}
				<div className="flex-grow flex flex-col gap-1">
					{thread.prompt && (
						<>
							<div className="text-sm text-content-secondary font-normal my-1">
								Prompt
							</div>
							<p className="text-sm text-content-secondary bg-surface-secondary leading-relaxed rounded-md p-3 overflow-auto m-0 text-pretty">
								{thread.prompt}
							</p>
						</>
					)}
				</div>

				{/* right column: details */}
				<PromptTable
					className="lg:max-w-64 flex-shrink-0"
					timestamp={new Date(thread.started_at)}
					model={thread.model}
					inputTokens={thread.token_usage.input_tokens}
					outputTokens={thread.token_usage.output_tokens}
					tokenUsageMetadata={thread.token_usage.metadata}
				/>
			</div>

			<BracketConnector
				firstRowHeight="60px"
				contentClassName="border border-dashed rounded-md my-4"
			>
				{/* Agentic loop */}
				<div className="flex flex-col lg:flex-row lg:items-center justify-between">
					<div>
						<CollapseButton
							isOpen={agenticLoopOpen}
							onClick={() => setAgenticLoopOpen(!agenticLoopOpen)}
						>
							<span className="text-sm">Agentic loop</span>
						</CollapseButton>
					</div>

					<AgenticLoopTable
						className="lg:max-w-64 flex-1 my-3 mx-2"
						duration={durationInMs}
						toolCalls={toolCalls}
						inputTokens={thread.token_usage.input_tokens}
						outputTokens={thread.token_usage.output_tokens}
					/>
				</div>

				{agenticLoopOpen && (
					<>
						{/* the little top rounded line above the thinking block */}
						<div className="border-0 border-t border-r border-solid rounded-tr-lg w-[calc(1rem+1px)] h-[20px]">
							{/* we need the 1px extra to line up with the left border on the other lines */}
						</div>

						{/* Agentic actions */}
						{thread.agentic_actions?.map((action, i) => (
							<AgenticActionItem key={`${thread.id}-${i}`} action={action} />
						))}

						{/* Agentic loop completed block */}
						<AgenticLoopCompletedBlock
							inputTokens={thread.token_usage.input_tokens}
							outputTokens={thread.token_usage.output_tokens}
						/>
					</>
				)}
			</BracketConnector>
		</>
	);
};

interface SessionTimelineProps {
	initiator: MinimalUser;
	threads: readonly AIBridgeThread[];
	hasNextPage: boolean;
	isFetchingNextPage: boolean;
	onFetchNextPage: () => void;
}

export const SessionTimeline: FC<SessionTimelineProps> = ({
	initiator,
	threads,
	hasNextPage,
	isFetchingNextPage,
	onFetchNextPage,
}) => {
	const sentinelRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const sentinel = sentinelRef.current;

		if (!sentinel || !hasNextPage) {
			return;
		}

		const observer = new IntersectionObserver(
			([div]) => {
				if (div.isIntersecting && hasNextPage && !isFetchingNextPage) {
					onFetchNextPage();
				}
			},
			{ rootMargin: "200px" },
		);

		observer.observe(sentinel);

		return () => {
			observer.disconnect();
		};
	}, [hasNextPage, isFetchingNextPage, onFetchNextPage]);

	return (
		<div className="relative">
			<div className="grid grid-cols-[16px_1rem_1px_1fr_auto_16px]">
				{/* row 1: session start */}
				<div className="row-start-1 col-start-2 relative h-10 py-1">
					<StatusIndicatorDot
						variant="inactive"
						className="absolute right-0 translate-x-1/2 translate-y-1/2"
					/>
				</div>
				<div className="row-start-1 col-start-4 col-span-2 flex items-center h-10">
					<span className="text-content-secondary ml-4 py-1 text-sm">
						Session started
					</span>
				</div>

				{/* row 2: vertical line */}
				<div className="row-start-2 col-start-3 border-0 border-l border-solid">
					{/* vertical line */}
				</div>

				{/* row 3: sized intentionally to create the visual space above the timeline border */}
				<div className="row-start-3 col-start-3 border-0 border-l border-t border-solid h-6">
					{/* vertical line */}
				</div>

				{/* row 3/4: AI Governance tooltip */}
				<div className="row-start-3 col-start-5 row-span-2 flex items-center text-sm text-content-secondary px-2 pt-1">
					AI Governance
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<InfoIcon className="size-icon-sm p-0.5 ml-1" />
							</TooltipTrigger>
							<TooltipContent
								className="max-w-64 text-sm"
								align="end"
								side="top"
							>
								<div className="text-content-secondary font-medium mb-1">
									Controls and logs AI tooling so AI use stays secure,
									compliant, and visible.
								</div>
								<div>
									<Link
										href={docs("/ai-coder/ai-governance")}
										target="_blank"
										className="text-sm"
									>
										More about AI Governance
									</Link>
								</div>
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</div>

				{/* row 4:  */}
				<div className="row-start-4 col-start-1 border-0 border-l border-t border-dashed border-surface-green rounded-tl-lg size-4">
					{/* top left rounded corner */}
				</div>
				<div className="row-start-4 col-start-2 border-0 border-t border-dashed border-surface-green">
					{/* horizontal border */}
				</div>
				<div className="row-start-4 col-start-3 border-0 border-l border-solid">
					{/* vertical line */}
				</div>
				<div className="row-start-4 col-start-4 border-0 border-t border-dashed">
					{/* horizontal border */}
				</div>
				<div className="row-start-4 col-start-6 border-0 border-r border-t border-dashed border-surface-green rounded-tr-lg size-4">
					{/* top right rounded corner */}
				</div>

				{/* row 5: threads */}
				<div className="row-start-5 col-start-1 border-0 border-l border-dashed border-surface-green">
					{/* left vertical line */}
				</div>
				<div className="row-start-5 col-start-2 col-span-4">
					{/* threads */}
					{threads.map((thread) => (
						<ThreadItem key={thread.id} thread={thread} initiator={initiator} />
					))}
					{/* infinite scroll sentinel — sits 200px below the last thread */}
					<div ref={sentinelRef} />
					{isFetchingNextPage && (
						<div className="flex items-center justify-center py-4 text-sm text-content-secondary">
							<Spinner loading size="sm" />
						</div>
					)}
				</div>
				<div className="row-start-5 col-start-6 border-0 border-r border-dashed border-surface-green">
					{/* right vertical line */}
				</div>

				{/* row 6: more design and session end */}
				<div className="row-start-6 col-start-1 border-0 border-l border-b border-dashed border-surface-green rounded-bl-lg size-4">
					{/* bottom left rounded corner */}
				</div>
				<div className="row-start-6 col-start-2 border-0 border-b border-dashed border-surface-green">
					{/* horizontal line */}
				</div>
				<div className="row-start-6 col-start-3 border-0 border-l border-solid">
					{/* vertical line */}
				</div>
				<div className="row-start-6 col-start-4 col-span-2 border-0 border-b border-dashed border-surface-green">
					{/* horizontal line */}
				</div>
				<div className="row-start-6 col-start-6 border-0 border-r border-b border-dashed border-surface-green rounded-br-lg size-4">
					{/* bottom right rounded corner */}
				</div>

				{/* row 7: sized intentionally to create the visual space below the timeline border */}
				<div className="row-start-7 col-start-3 border-0 border-l border-t border-solid h-4">
					{/* vertical line */}
				</div>

				{/* row 8: session start */}
				<div className="row-start-8 col-start-2 relative">
					<StatusIndicatorDot
						variant="success"
						className="absolute right-0 translate-x-1/2 translate-y-1/2"
					/>
				</div>
				<div className="row-start-8 col-start-4 flex items-center">
					<span className="text-content-success ml-4 text-sm py-1">
						Session completed
					</span>
				</div>
			</div>
		</div>
	);
};
