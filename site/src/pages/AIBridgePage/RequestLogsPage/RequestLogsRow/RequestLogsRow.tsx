import type { AIBridgeInterception } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
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
import { type FC, Fragment, useState } from "react";
import { cn } from "utils/cn";
import { humanDuration } from "utils/time";

type RequestLogsRowProps = {
	interception: AIBridgeInterception;
};

const customisedDateLocale: Intl.DateTimeFormatOptions = {
	// Hide the year from the date
	year: undefined,
	// Show the month as a short name
	month: "short",
	day: "numeric",
	hour: "numeric",
	minute: "2-digit",
	second: "2-digit",
	hour12: true,
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
	const duration =
		interception.ended_at &&
		Math.max(
			0,
			new Date(interception.ended_at).getTime() -
				new Date(interception.started_at).getTime(),
		);

	return (
		<>
			<TableRow
				className="select-none cursor-pointer hover:bg-surface-secondary"
				onClick={() => setIsOpen(!isOpen)}
			>
				<TableCell className="w-48 whitespace-nowrap">
					<div
						className={cn([
							"flex items-center gap-2",
							isOpen && "text-content-primary",
						])}
					>
						{isOpen ? (
							<ChevronDownIcon className="size-icon-xs" />
						) : (
							<ChevronRightIcon className="size-icon-xs" />
						)}
						<span className="sr-only">({isOpen ? "Hide" : "Show more"})</span>
						{new Date(interception.started_at).toLocaleString(
							undefined,
							customisedDateLocale,
						)}
					</div>
				</TableCell>
				<TableCell className="w-48">
					<div className="flex items-center gap-3">
						<Avatar
							fallback={interception.initiator.username}
							src={interception.initiator.avatar_url}
							size={"lg"}
						/>
						<div className="font-medium">{interception.initiator.username}</div>
					</div>
				</TableCell>
				<TableCell>{firstPrompt?.prompt}</TableCell>
				<TableCell className="w-32">
					<div className="flex items-center">
						<Badge className="gap-0 rounded-e-none">
							<ArrowDownIcon className="size-icon-lg flex-shrink-0" />
							<span className="truncate min-w-0 w-full">{inputTokens}</span>
						</Badge>
						<Badge className="gap-0 bg-surface-tertiary rounded-s-none">
							<ArrowUpIcon className="size-icon-lg flex-shrink-0" />
							<span className="truncate min-w-0 w-full">{outputTokens}</span>
						</Badge>
					</div>
				</TableCell>
				<TableCell className="w-40 max-w-40">
					<div className="w-full min-w-0 overflow-hidden">
						<Badge className="gap-2 w-full">
							<div className="size-[18px] bg-red-500 flex-shrink-0"></div>
							<span className="truncate min-w-0 w-full">
								{interception.model}
							</span>
						</Badge>
					</div>
				</TableCell>
				<TableCell className="w-32 text-center">{toolCalls}</TableCell>
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

								{(duration || duration === 0) && (
									<>
										<dt>Duration:</dt>
										<dd title={duration.toString()} data-chromatic="ignore">
											{humanDuration(duration)}
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
										{interception.user_prompts.map((prompt) => (
											<Fragment key={prompt.id}>{prompt.prompt}</Fragment>
										))}
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
