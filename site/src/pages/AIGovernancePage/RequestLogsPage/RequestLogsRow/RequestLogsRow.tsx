import type { AIBridgeInterception } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { TableCell, TableRow } from "components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	ArrowDownIcon,
	ArrowUpIcon,
	ChevronDownIcon,
	ChevronRightIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";

type RequestLogsRowProps = {
	interception: AIBridgeInterception;
};

export const RequestLogsRow: FC<RequestLogsRowProps> = ({ interception }) => {
	const [isOpen, setIsOpen] = useState(false);

	const [firstPrompt] = interception.user_prompts;

	const inputTokens = interception.token_usages.reduce(
		(acc, tokenUsage) => acc + tokenUsage.input_tokens,
		0,
	);
	const outputTokens = interception.token_usages.reduce(
		(acc, tokenUsage) => acc + tokenUsage.output_tokens,
		0,
	);
	const toolCalls = interception.tool_usages.length;

	return (
		<>
			<TableRow
				className="select-none cursor-pointer hover:bg-surface-secondary"
				onClick={() => setIsOpen(!isOpen)}
			>
				<TableCell>
					<div
						className={cn([
							"flex items-center gap-2",
							isOpen && "text-content-primary",
						])}
					>
						{isOpen ? (
							<ChevronDownIcon size={16} />
						) : (
							<ChevronRightIcon size={16} />
						)}
						<span className="sr-only">({isOpen ? "Hide" : "Show more"})</span>
						{new Date(interception.started_at).toLocaleString()}
					</div>
				</TableCell>
				<TableCell>
					<div className="flex items-center gap-3">
						<Avatar
							fallback={interception.initiator.username}
							src={interception.initiator.avatar_url}
						/>
						<div className="font-medium">{interception.initiator.username}</div>
					</div>
				</TableCell>
				<TableCell>{firstPrompt?.prompt}</TableCell>
				<TableCell>
					<div className="flex items-center gap-4">
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<div className="flex items-center gap-1">
										<ArrowDownIcon size={16} />
										<div>{inputTokens}</div>
									</div>
								</TooltipTrigger>
								<TooltipContent>Input Tokens</TooltipContent>
							</Tooltip>
						</TooltipProvider>
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<div className="flex items-center gap-1">
										<ArrowUpIcon size={16} />
										<div>{outputTokens}</div>
									</div>
								</TooltipTrigger>
								<TooltipContent>Output Tokens</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					</div>
				</TableCell>
				<TableCell>{toolCalls}</TableCell>
			</TableRow>
			{isOpen && (
				<TableRow>
					<TableCell colSpan={999} className="p-4 border-t-0">
						<div className="flex flex-col gap-4">
							<dl
								className={cn([
									"text-xs text-content-secondary",
									"m-0 grid grid-cols-[auto_1fr] gap-x-4 items-center",
									"[&_dd]:text-content-primary [&_dd]:font-mono [&_dd]:leading-[22px] [&_dt]:font-medium",
								])}
							>
								<dt>Request ID:</dt>
								<dd data-chromatic="ignore">{interception.id}</dd>

								<dt>Start Time:</dt>
								<dd data-chromatic="ignore">
									{new Date(interception.started_at).toLocaleString()}
								</dd>

								{interception.ended_at && (
									<>
										<dt>End Time:</dt>
										<dd data-chromatic="ignore">
											{new Date(interception.ended_at).toLocaleString()}
										</dd>
									</>
								)}

								<dt>Initiator:</dt>
								<dd data-chromatic="ignore">
									{interception.initiator.username}
								</dd>

								<dt>Model:</dt>
								<dd data-chromatic="ignore">{interception.model}</dd>

								<dt>Input Tokens:</dt>
								<dd data-chromatic="ignore">{inputTokens}</dd>

								<dt>Output Tokens:</dt>
								<dd data-chromatic="ignore">{outputTokens}</dd>

								<dt>Tool Calls:</dt>
								<dd data-chromatic="ignore">
									{interception.tool_usages.length}
								</dd>
							</dl>

							{interception.user_prompts.length > 0 && (
								<div className="flex flex-col gap-2">
									<div>Prompts</div>
									<div
										className="bg-surface-secondary rounded-md p-4"
										data-chromatic="ignore"
									>
										{interception.user_prompts.map((prompt) => prompt.prompt)}
									</div>
								</div>
							)}

							{interception.tool_usages.length > 0 && (
								<div className="flex flex-col gap-2">
									<div>Tool Usages</div>
									<div
										className="bg-surface-secondary rounded-md p-4"
										data-chromatic="ignore"
									>
										{interception.tool_usages.map((toolUsage) => {
											return (
												<dl
													key={toolUsage.id}
													className={cn([
														"text-xs text-content-secondary",
														"m-0 grid grid-cols-[auto_1fr] gap-x-4 items-center",
														"[&_dt]:text-content-primary [&_dd]:font-mono [&_dt]:leading-[22px] [&_dt]:font-medium",
													])}
												>
													<dt>{toolUsage.tool}</dt>
													<dd>
														<div className="flex flex-col gap-2">
															<div>{toolUsage.input}</div>
															{toolUsage.invocation_error && (
																<div className="text-content-destructive">
																	{toolUsage.invocation_error}
																</div>
															)}
														</div>
													</dd>
												</dl>
											);
										})}
									</div>
								</div>
							)}
						</div>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};
