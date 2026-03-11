import type { AIBridgeSessionResponse } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { Link } from "components/Link/Link";
import { Spinner } from "components/Spinner/Spinner";
import { StatusIndicatorDot } from "components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ChevronDownIcon, ChevronRightIcon, InfoIcon } from "lucide-react";
import { type FC, useState } from "react";
import { docs } from "utils/docs";
import {
	AgenticLoopDetailsTable,
	PromptDetailsTable,
	TokenBadges,
} from "./AISessionDetailsTable";
import { formatToolCalInput } from "../utils";

type Thread = AIBridgeSessionResponse["threads"][number];
type AgenticAction = Thread["agentic_actions"][number];

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
	>
		{isOpen ? (
			<ChevronDownIcon className="size-3.5 flex-shrink-0" />
		) : (
			<ChevronRightIcon className="size-3.5 flex-shrink-0" />
		)}
		{children}
	</Button>
);

interface ThinkingBlockProps {
	text: string;
}

interface AgenticActionItemProps {
	action: AgenticAction;
}

const AgenticActionItem: FC<AgenticActionItemProps> = ({ action }) => {
	const [toolCallOpen, setToolCallOpen] = useState(true);
	const [agenticLoopOpen, setAgenticLoopOpen] = useState(true);

	// FIXME the designs make it look like it can be multple tool calls per
	// action, but currently the API only supports one. Need to confirm if we
	// want to update the API or adjust the design.
	const { tool_call } = action;

	return (
		<>
			{/* the little top rounded line above the thinking block */}
			<div className="border-0 border-t border-r border-solid border-surface-secondary rounded-tr-lg w-[calc(1rem+1px)] h-[20px]">
				{/* we need the 1px extra to line up with the left border on the other lines */}
			</div>
			{/* thinking blocks */}
			{action.thinking.map((t) => (
				<div
					key={t.text}
					className="grid grid-cols-[1rem_1rem_1fr] grid-rows-[2rem_auto]"
				>
					<div className="row-start-1 col-start-2 border-0 border-b border-l border-solid border-surface-secondary rounded-bl-lg">
						{/* top rounded line */}
					</div>
					<div className="row-start-2 col-start-2 border-0 border-t border-l border-solid border-surface-secondary rounded-tl-lg">
						{/* bottom rounded line */}
					</div>
					<div className="row-start-1 col-start-3 row-span-2 mt-5 pl-2 text-sm text-content-secondary">
						<div className="flex items-center">
							<Spinner loading={true} size="sm" />
							<span className="font-mono ml-2">Thinking...</span>
						</div>
						<p>{t.text}</p>
					</div>
				</div>
			))}

			{/* tool call block */}
			<div className="grid grid-cols-[1rem_1rem_1fr] grid-rows-[2rem_auto]">
				<div className="row-start-1 col-start-2 border-0 border-b border-l border-solid border-surface-secondary rounded-bl-lg">
					{/* top rounded line */}
				</div>
				<div className="row-start-2 col-start-2 border-0 border-t border-l border-solid border-surface-secondary rounded-tl-lg">
					{/* bottom rounded line */}{" "}
				</div>
				<div className="row-start-1 col-start-3 row-span-2 mt-3 border border-solid border-surface-secondary rounded-md">
					<div className="flex items-center">
						<CollapseButton
							isOpen={toolCallOpen}
							onClick={() => setToolCallOpen(!toolCallOpen)}
						>
							<span>Tool call</span>
							<Badge size="xs" className="font-mono">
								{tool_call.tool}
							</Badge>
						</CollapseButton>
					</div>
					{toolCallOpen && (
						<>
							<div className="mt-2 ml-5 flex flex-col gap-2 w-1/2 text-xs text-content-secondary">
								<div className="flex items-center justify-between">
									<span className="font-medium">In / out tokens</span>
									<TokenBadges inputTokens={123} outputTokens={456} />
								</div>
								<div className="flex items-center justify-between">
									<span className="font-medium">MCP server</span>
									<span className="font-mono truncate">
										{tool_call.server_url}
									</span>
								</div>
							</div>
							<pre className="bg-surface-secondary rounded-md m-4 p-4 text-xs font-mono text-content-primary overflow-x-auto m-0">
								{formatToolCalInput(tool_call.input)}
							</pre>
						</>
					)}
				</div>
			</div>

			{/* Agentic loop over block */}
			<div className="grid grid-cols-[1rem_1rem_1fr] grid-rows-[2rem_auto]">
				<div className="row-start-1 col-start-2 border-0 border-b border-l border-solid border-surface-secondary rounded-bl-lg">
					{/* top rounded line */}
				</div>
				<div className="row-start-1 col-start-3 row-span-2 mt-3 border border-solid border-surface-secondary rounded-md mb-4">
					<div className="flex items-center">
						<CollapseButton
							isOpen={agenticLoopOpen}
							onClick={() => setAgenticLoopOpen(!agenticLoopOpen)}
						>
							<span>Agentic loop completed</span>
						</CollapseButton>
					</div>
					{agenticLoopOpen && (
						<div className="mb-4 ml-5 flex flex-col gap-2 w-1/2 text-xs text-content-secondary">
							<div className="flex items-center justify-between">
								<span className="font-medium">In / out tokens</span>
								<TokenBadges inputTokens={123} outputTokens={456} />
							</div>
							<div className="flex items-center justify-between">
								<span className="font-medium">MCP server</span>
								<span className="font-mono truncate">
									{tool_call.server_url}
								</span>
							</div>
						</div>
					)}
				</div>
			</div>
		</>
	);
};

interface ThreadItemProps {
	thread: Thread;
	initiator: AIBridgeSessionResponse["initiator"];
}

const ThreadItem: FC<ThreadItemProps> = ({ thread, initiator }) => {
	const [agenticLoopOpen, setAgenticLoopOpen] = useState(true);
	const hasAgenticActions = thread.agentic_actions.length > 0;

	// Use the first tool call's timestamp as the thread timestamp.
	const timestamp = thread.agentic_actions[0]?.tool_call.created_at;

	// FIXME how to get the end time of a tool call?
	const duration = 12_345; // in ms

	// TODO double check if summing tokens like this is correct
	const inputTokens =
		thread.token_usage.input_tokens +
		thread.agentic_actions.reduce(
			(sum, action) => sum + action.token_usage.input_tokens,
			0,
		);
	const outputTokens =
		thread.token_usage.output_tokens +
		thread.agentic_actions.reduce(
			(sum, action) => sum + action.token_usage.output_tokens,
			0,
		);

	return (
		<>
			<div className="border border-border border-solid rounded-md flex p-4">
				{/* left column: avatar and username */}
				<div className="flex flex-row items-items-start gap-2">
					<Avatar
						src={initiator.avatar_url}
						fallback={initiator.name ?? initiator.username}
						size="sm"
						className="flex-shrink-0"
					/>
					<span className="text-sm text-content-primary">
						{initiator.username}
					</span>
				</div>

				{/* center column: prompt */}
				<div className="flex-grow px-6">
					<div className="text-sm text-content-secondary">Prompt</div>
					<p className="text-sm text-content-primary bg-surface-secondary leading-relaxed rounded-md p-3 max-h-48 overflow-auto m-0">
						{thread.prompt}
						Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do
						eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim
						ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut
						aliquip ex ea commodo consequat. Duis aute irure dolor in
						reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla
						pariatur. Excepteur sint occaecat cupidatat non proident, sunt in
						culpa qui officia deserunt mollit anim id est laborum.
					</p>
				</div>

				{/* right column: details */}
				<PromptDetailsTable
					timestamp={timestamp ? new Date(timestamp) : new Date()}
					model={thread.model}
					inputTokens={thread.token_usage.input_tokens}
					outputTokens={thread.token_usage.output_tokens}
				/>
			</div>

			<div className="grid grid-cols-[1rem_1rem_1fr] grid-rows-[60px_auto]">
				<div className="row-start-1 col-start-2 border-0 border-l border-b border-solid border-surface-secondary rounded-bl-lg">
					{/* vertical line */}
				</div>
				<div className="row-start-2 col-start-2 border-0 border-l border-t border-solid border-surface-secondary rounded-tl-lg">
					{/* vertical line */}
				</div>
				<div className="row-start-1 col-start-3 row-span-2 border border-border border-dashed rounded-md my-4">
					{/* Agentic loop */}
					{hasAgenticActions && (
						<div className="flex flex-col">
							<div className="flex items-center justify-between flex-wrap">
								<CollapseButton
									isOpen={agenticLoopOpen}
									onClick={() => setAgenticLoopOpen(!agenticLoopOpen)}
								>
									Agentic loop
								</CollapseButton>

								<AgenticLoopDetailsTable
									className="m-4"
									duration={duration}
									toolCalls={thread.agentic_actions.length}
									inputTokens={inputTokens}
									outputTokens={outputTokens}
								/>
							</div>

							{agenticLoopOpen &&
								thread.agentic_actions.map((action) => (
									<AgenticActionItem
										key={action.tool_call.id}
										action={action}
									/>
								))}
						</div>
					)}
				</div>
			</div>
		</>
	);
};

export interface AISessionTimelineProps {
	session: AIBridgeSessionResponse;
}

export const AISessionTimeline: FC<AISessionTimelineProps> = ({ session }) => {
	return (
		<div className="grid grid-cols-[20px_1rem_1px_1fr_auto_20px]">
			{/* row 1: session start */}
			<div className="row-start-1 col-start-2 relative">
				<StatusIndicatorDot
					variant="inactive"
					size="sm"
					className="absolute right-0 translate-x-1/2 translate-y-1/2"
				/>
			</div>
			<div className="row-start-1 col-start-4 flex items-center">
				<span className="text-content-secondary ml-4">Session started</span>
			</div>

			{/* row 2: vertical line and timeline sort dropdown */}
			<div className="row-start-2 col-start-3 border-0 border-l border-solid border-surface-secondary">
				{/* vertical line */}
			</div>
			<div className="row-start-2 col-start-4 col-span-2 text-right">
				{/* TODO */}
				<div className="border border-solid border-border rounded-md inline-flex items-center text-xs text-content-secondary px-2 py-1">
					TODO sort action dropdown
				</div>
			</div>

			{/* row 3: sized intentionally to create the visual space above the timeline border */}
			<div className="row-start-3 col-start-3 border-0 border-l border-t border-solid border-surface-secondary h-[20px]">
				{/* vertical line */}
			</div>

			{/* row 3/4: AI Governance tooltip */}
			<div className="row-start-3 col-start-5 row-span-2 flex items-center text-xs text-content-secondary px-2">
				AI Governance
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<InfoIcon className="size-icon-sm ml-2" />
						</TooltipTrigger>
						<TooltipContent className="max-w-64" align="end" side="top">
							<div className="text-sm text-content-primary font-medium mb-1">
								Controls and logs AI tooling so AI use stays secure, compliant,
								and visible.
							</div>
							<div className="text-sm text-content-secondary">
								<Link href={docs("/ai-coder/ai-governance")} target="_blank">
									More about AI Governance
								</Link>
							</div>
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</div>

			{/* row 4:  */}
			<div className="row-start-4 col-start-1 border-0 border-l border-t border-dashed border-border-success/40 rounded-tl-full w-[20px] h-[20px]">
				{/* top left rounded corner */}
			</div>
			<div className="row-start-4 col-start-2 border-0 border-t border-dashed border-border-success/40">
				{/* horizontal border */}
			</div>
			<div className="row-start-4 col-start-3 border-0 border-l border-solid border-surface-secondary">
				{/* vertical line */}
			</div>
			<div className="row-start-4 col-start-4 border-0 border-t border-dashed border-border-success/40">
				{/* horizontal border */}
			</div>
			<div className="row-start-4 col-start-6 border-0 border-r border-t border-dashed border-border-success/40 rounded-tr-full w-[20px] h-[20px]">
				{/* top right rounded corner */}
			</div>

			{/* row 5: threads */}
			<div className="row-start-5 col-start-1 border-0 border-l border-dashed border-border-success/40">
				{/* left vertical line */}
			</div>
			<div className="row-start-5 col-start-2 col-span-4">
				{/* threads */}
				{session.threads.map((thread) => (
					<ThreadItem
						key={thread.id}
						thread={thread}
						initiator={session.initiator}
					/>
				))}
			</div>
			<div className="row-start-5 col-start-6 border-0 border-r border-dashed border-border-success/40">
				{/* right vertical line */}
			</div>

			{/* row 6: more design and session end */}
			<div className="row-start-6 col-start-1 border-0 border-l border-b border-dashed border-border-success/40 rounded-bl-full w-[20px] h-[20px]">
				{/* bottom left rounded corner */}
			</div>
			<div className="row-start-6 col-start-2 border-0 border-b border-dashed border-border-success/40">
				{/* horizontal line */}
			</div>
			<div className="row-start-6 col-start-3 border-0 border-l border-solid border-surface-secondary">
				{/* vertical line */}
			</div>
			<div className="row-start-6 col-start-4 col-span-2 border-0 border-b border-dashed border-border-success/40">
				{/* horizontal line */}
			</div>
			<div className="row-start-6 col-start-6 border-0 border-r border-b border-dashed border-border-success/40 rounded-br-full w-[20px] h-[20px]">
				{/* bottom right rounded corner */}
			</div>

			{/* row 7: sized intentionally to create the visual space below the timeline border */}
			<div className="row-start-7 col-start-3 border-0 border-l border-t border-solid border-surface-secondary h-[20px]">
				{/* vertical line */}
			</div>

			{/* row 8: session start */}
			<div className="row-start-8 col-start-2 relative">
				<StatusIndicatorDot
					variant="success"
					size="sm"
					className="absolute right-0 translate-x-1/2 translate-y-1/2"
				/>
			</div>
			<div className="row-start-8 col-start-4 flex items-center">
				<span className="text-success ml-4">Session ended</span>
			</div>
		</div>
	);
};
