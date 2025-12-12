import type { AIBridgeInterception } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
import { AnthropicIcon } from "components/Icons/AnthropicIcon";
import { OpenAiIcon } from "components/Icons/OpenAiIcon";
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
	CircleQuestionMarkIcon,
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
	hour: "2-digit",
	minute: "2-digit",
	second: "2-digit",
	hour12: true,
};

export const RequestLogsRowProviderIcon = ({
	provider,
	...props
}: {
	provider: string;
} & React.ComponentProps<"svg">) => {
	const iconClassName = "size-icon-sm flex-shrink-0";
	switch (provider) {
		case "openai":
			return (
				<OpenAiIcon className={cn(iconClassName, props.className)} {...props} />
			);
		case "anthropic":
			return (
				<AnthropicIcon
					className={cn(iconClassName, props.className)}
					{...props}
				/>
			);
		default:
			return (
				<CircleQuestionMarkIcon
					className={cn(iconClassName, props.className)}
					{...props}
				/>
			);
	}
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
				className="select-none cursor-pointer"
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
				<TableCell className="w-48 max-w-48">
					<div className="w-full min-w-0 overflow-hidden">
						<div className="flex items-center gap-3 min-w-0">
							<Avatar
								fallback={interception.initiator.username}
								src={interception.initiator.avatar_url}
								size={"lg"}
								className="flex-shrink-0"
							/>
							<div className="font-medium truncate min-w-0 flex-1 overflow-hidden">
								{interception.initiator.username}
							</div>
						</div>
					</div>
				</TableCell>
				<TableCell className="min-w-0">
					{/*
						This looks scary, but essentially what we're doing is ensuring that the
						prompt is truncated and won't escape its bounding container with an `absolute`.

						Alternatively we could use a `table-fixed` table, but that would break worse
						on mobile with the `min-w-0`. This is a bit of a hack, but it works.
					*/}
					<div className="w-full h-4 min-w-48 relative">
						<div className="absolute inset-0 leading-none overflow-hidden truncate">
							{firstPrompt?.prompt}
						</div>
					</div>
				</TableCell>
				<TableCell className="w-32">
					<div className="flex items-center">
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<Badge className="gap-0 rounded-e-none">
										<ArrowDownIcon className="size-icon-lg flex-shrink-0" />
										<span className="truncate min-w-0 w-full">
											{inputTokens}
										</span>
									</Badge>
								</TooltipTrigger>
								<TooltipContent>{inputTokens} Input Tokens</TooltipContent>
							</Tooltip>
						</TooltipProvider>
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<Badge className="gap-0 bg-surface-tertiary rounded-s-none">
										<ArrowUpIcon className="size-icon-lg flex-shrink-0" />
										<span className="truncate min-w-0 w-full">
											{outputTokens}
										</span>
									</Badge>
								</TooltipTrigger>
								<TooltipContent>{outputTokens} Output Tokens</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					</div>
				</TableCell>
				<TableCell className="w-40 max-w-40">
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<div className="w-full min-w-0 overflow-hidden">
									<Badge className="gap-0.5 w-full">
										<div className="flex-shrink-0 flex items-center">
											<RequestLogsRowProviderIcon
												provider={interception.provider}
											/>
										</div>
										<span className="truncate min-w-0 w-full">
											{interception.model}
										</span>
									</Badge>
								</div>
							</TooltipTrigger>
							<TooltipContent>{interception.model}</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</TableCell>
				<TableCell className="w-32 text-center">{toolCalls}</TableCell>
			</TableRow>
			{isOpen && (
				<TableRow>
					<TableCell colSpan={999} className="p-4 border-t-0">
						<div className="flex flex-col gap-6">
							<dl
								className={cn([
									"text-xs text-content-secondary",
									"m-0 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 items-center",
									"[&_dd]:text-content-primary [&_dd]:font-mono [&_dd]:leading-[22px] [&_dt]:font-medium",
								])}
							>
								<dt>Request ID:</dt>
								<dd data-chromatic="ignore">{interception.id}</dd>

								<dt>Start Time:</dt>
								<dd data-chromatic="ignore">
									{new Date(interception.started_at).toLocaleString(
										undefined,
										customisedDateLocale,
									)}
								</dd>

								{interception.ended_at && (
									<>
										<dt>End Time:</dt>
										<dd data-chromatic="ignore">
											{new Date(interception.started_at).toLocaleString(
												undefined,
												customisedDateLocale,
											)}
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
								<dd
									data-chromatic="ignore"
									className="flex items-center gap-1.5"
								>
									<Avatar
										fallback={interception.initiator.username}
										src={interception.initiator.avatar_url}
										size={"sm"}
										className="flex-shrink-0"
									/>
									<span className="truncate min-w-0 w-full">
										{interception.initiator.username}
									</span>
								</dd>

								<dt>Model:</dt>
								<dd data-chromatic="ignore">
									<Badge className="gap-0.5">
										<div className="flex-shrink-0 flex items-center">
											<RequestLogsRowProviderIcon
												provider={interception.provider}
												className="size-icon-xs"
											/>
										</div>
										<span className="truncate min-w-0 w-full text-2xs">
											{interception.model}
										</span>
									</Badge>
								</dd>

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
