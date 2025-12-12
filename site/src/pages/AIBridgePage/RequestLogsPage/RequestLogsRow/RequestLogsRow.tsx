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
import { type FC, Fragment, useState } from "react";
import { cn } from "utils/cn";
import { humanDuration } from "utils/time";

type RequestLogsRowProps = {
	interception: AIBridgeInterception;
};

type TokenUsageMetadataMerged =
	| null
	| Record<string, unknown>
	| Array<Record<string, unknown>>;

/**
 * This function merges multiple objects with the same keys into a single object.
 * It's super unconventional, but it's only a temporary workaround until we
 * structure our metadata field for rendering in the UI.
 * @param objects - The objects to merge.
 * @returns The merged object.
 */
function tokenUsageMetadataMerge(
	...objects: Array<
		AIBridgeInterception["token_usages"][number]["metadata"] | null
	>
): TokenUsageMetadataMerged {
	const validObjects = objects.filter((obj) => obj !== null);

	// Filter out empty objects
	const nonEmptyObjects = validObjects.filter(
		(obj) => Object.keys(obj).length > 0,
	);
	if (nonEmptyObjects.length === 0) {
		return null;
	}

	const allKeys = new Set(nonEmptyObjects.flatMap((obj) => Object.keys(obj)));
	const commonKeys = Array.from(allKeys).filter((key) =>
		nonEmptyObjects.every((obj) => key in obj),
	);
	if (commonKeys.length === 0) {
		return null;
	}

	// Check for unresolvable conflicts: values that aren't all numeric or all
	// the same.
	for (const key of allKeys) {
		const objectsWithKey = nonEmptyObjects.filter((obj) => key in obj);
		if (objectsWithKey.length > 1) {
			const values = objectsWithKey.map((obj) => obj[key]);
			const allNumeric = values.every((v: unknown) => typeof v === "number");
			const allSame = new Set(values).size === 1;
			if (!allNumeric && !allSame) {
				return nonEmptyObjects;
			}
		}
	}

	// Merge common keys: sum numeric values, preserve identical values, mark
	// conflicts as null.
	const result: Record<string, unknown> = {};
	for (const key of commonKeys) {
		const values = nonEmptyObjects.map((obj) => obj[key]);
		const allNumeric = values.every((v: unknown) => typeof v === "number");
		const allSame = new Set(values).size === 1;

		if (allNumeric) {
			result[key] = values.reduce((acc, v) => acc + (v as number), 0);
		} else if (allSame) {
			result[key] = values[0];
		} else {
			result[key] = null;
		}
	}

	// Add non-common keys from the first object that has them.
	for (const obj of nonEmptyObjects) {
		for (const key of Object.keys(obj)) {
			if (!commonKeys.includes(key) && !(key in result)) {
				result[key] = obj[key];
			}
		}
	}

	// If any conflicts were marked, return original objects.
	return Object.values(result).some((v: unknown) => v === null)
		? nonEmptyObjects
		: result;
}

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

	const tokenUsagesMetadata = tokenUsageMetadataMerge(
		...interception.token_usages.map((tokenUsage) => tokenUsage.metadata),
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
				<TableCell>
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
										<ArrowDownIcon className="size-icon-xs" />
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
										<ArrowUpIcon className="size-icon-xs" />
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

							{tokenUsagesMetadata !== null && (
								<div className="flex flex-col gap-2">
									<div>Token Usage Metadata</div>
									<div className="bg-surface-secondary rounded-md p-4">
										<pre>{JSON.stringify(tokenUsagesMetadata, null, 2)}</pre>
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
