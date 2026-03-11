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
		className="border-none bg-transparent text-content-secondary flex items-center gap-2"
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

const ThinkingBlock: FC<ThinkingBlockProps> = ({ text }) => (
	<div className="border-0 border-l border-solid border-border pl-3 text-sm text-content-secondary">
		<div className="flex items-center">
			<Spinner loading={true} size="sm" />
			<span className="font-mono ml-2">Thinking...</span>
		</div>
		<p>{text}</p>
	</div>
);

interface AgenticActionItemProps {
	action: AgenticAction;
}

const AgenticActionItem: FC<AgenticActionItemProps> = ({ action }) => {
	const [toolCallOpen, setToolCallOpen] = useState(true);

	const { tool_call } = action;

	return (
		<div className="flex flex-col items-start justify-start gap-2">
			{/* Thinking blocks */}
			{action.thinking.map((t) => (
				<ThinkingBlock key={t.text} text={t.text} />
			))}

			<div className="w-full border border-solid border-border rounded-md">
				<div className="flex items-start justify-between gap-4">
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
			<div className="border border-border border-solid rounded-md flex p-4 m-4">
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

			<div className="border border-border border-dashed rounded-md p-4 m-4">
				{/* Agentic loop */}
				{hasAgenticActions && (
					<div className="ml-9 flex flex-col gap-3">
						<div className="flex items-center justify-between">
							<CollapseButton
								isOpen={agenticLoopOpen}
								onClick={() => setAgenticLoopOpen(!agenticLoopOpen)}
							>
								Agentic loop
							</CollapseButton>

							<AgenticLoopDetailsTable
								duration={duration}
								toolCalls={thread.agentic_actions.length}
								inputTokens={inputTokens}
								outputTokens={outputTokens}
							/>
						</div>

						{agenticLoopOpen && (
							<div className="pl-4 flex flex-col gap-4">
								{thread.agentic_actions.map((action) => (
									<AgenticActionItem
										key={action.tool_call.id}
										action={action}
									/>
								))}
							</div>
						)}
					</div>
				)}
			</div>
		</>
	);
};

const SessionHeader: FC = () => (
	<>
		<div className="border-l border-border pl-4 ml-4 flex items-center">
			<StatusIndicatorDot variant="inactive" size="sm" />
			<span className="text-content-secondary ml-4">Session started</span>
		</div>
		<div className="flex justify-end">
			<span className="text-sm text-content-secondary flex items-center">
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
			</span>
		</div>
	</>
);

const SessionFooter: FC = () => (
	<div className="border-l border-border pl-4 ml-4 flex items-center">
		<StatusIndicatorDot variant="success" size="sm" />
		<span className="text-sm text-content-success ml-4">Session completed</span>
	</div>
);

export interface AISessionTimelineProps {
	session: AIBridgeSessionResponse;
}

export const AISessionTimeline: FC<AISessionTimelineProps> = ({ session }) => {
	return (
		<div className="flex flex-col gap-4">
			<SessionHeader />
			<div className="border border-solid border-border-success/40 border-dashed rounded-md">
				{session.threads.map((thread) => (
					<ThreadItem
						key={thread.id}
						thread={thread}
						initiator={session.initiator}
					/>
				))}
			</div>
			<SessionFooter />
		</div>
	);
};
