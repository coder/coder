import { BotIcon, InfoIcon } from "lucide-react";
import { type ComponentType, useContext } from "react";
import { useQuery } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { preferenceSettings } from "#/api/queries/users";
import { Badge } from "#/components/Badge/Badge";
import {
	ReasoningDisclosure,
	ResponseBlock,
} from "../../ChatConversation/MessageText";
import { Response } from "../Response";
import { ACPContext, acpSessionKey } from "./ACPContext";
import { type ACPToolName, ACPToolNames } from "./acpToolNames";
import { parseACPTranscript } from "./acpTranscript";
import type { Tool } from "./Tool";
import { ToolCall } from "./ToolCall";
import { asString, parseArgs } from "./utils";

type NestedToolProps = Required<
	Pick<
		React.ComponentProps<typeof Tool>,
		| "organizationId"
		| "name"
		| "status"
		| "args"
		| "result"
		| "isError"
		| "subagentTitles"
		| "subagentVariants"
		| "mcpServers"
		| "shellToolDisplayMode"
		| "codeDiffDisplayMode"
	>
>;

type ACPToolProps = Omit<NestedToolProps, "name"> & {
	name: ACPToolName;
	ToolComponent: ComponentType<NestedToolProps>;
};

const verbs: Record<ACPToolProps["name"], readonly [string, string]> = {
	[ACPToolNames.SpawnAgent]: ["Spawning", "Spawned"],
	[ACPToolNames.WaitAgent]: ["Waiting for", "Waited for"],
	[ACPToolNames.MessageAgent]: ["Messaging", "Messaged"],
	[ACPToolNames.InterruptAgent]: ["Interrupting", "Interrupted"],
};

type AgentContentCardProps = {
	children: React.ReactNode;
};

const AgentContentCard: React.FC<AgentContentCardProps> = ({ children }) => {
	return (
		<div className="mt-2 rounded-lg border border-solid border-border [&_[data-streamdown=table-wrapper]]:border-0 [&_[data-streamdown=table-wrapper]]:bg-transparent [&_[data-streamdown=table-wrapper]]:p-0">
			<div className="p-3">{children}</div>
		</div>
	);
};

/** Renders ACP lifecycle calls and their nested agent activity. */
export const ACPTool: React.FC<ACPToolProps> = ({
	ToolComponent,
	organizationId,
	name,
	status,
	args,
	result,
	isError,
	subagentTitles,
	subagentVariants,
	mcpServers,
	shellToolDisplayMode,
	codeDiffDisplayMode,
}) => {
	const sessions = useContext(ACPContext);
	const preferences = useQuery(preferenceSettings());
	const input = parseArgs(args);
	const output = parseArgs(result);
	const session = acpSessionKey(args, result);
	const descriptor = sessions.get(session);
	const agentName =
		asString(output?.harness_display_name) ||
		descriptor?.displayName ||
		asString(output?.harness_id) ||
		asString(input?.harness_id) ||
		"ACP agent";
	const prompt = asString(input?.prompt) || asString(input?.message);
	const title = descriptor?.prompt || asString(input?.prompt) || "ACP agent";
	const action = verbs[name][status === "running" ? 0 : 1];
	const rawError = output?.error || (isError ? result : "");
	const error = getErrorMessage(parseArgs(rawError) ?? rawError, "");
	const blocks = parseACPTranscript(output?.messages);
	const preview = asString(output?.output) || asString(output?.response);
	const waiting = name === ACPToolNames.WaitAgent;
	const hasContent = waiting || Boolean(prompt || error);
	return (
		<ToolCall.Root
			status={status}
			isError={isError || Boolean(error)}
			errorMessage={error}
			hasContent={hasContent}
			defaultExpanded
		>
			<ToolCall.HeaderLayout>
				<ToolCall.HeaderButton>
					<BotIcon aria-hidden className="size-4 shrink-0" />
					<ToolCall.Label>
						{action}{" "}
						<Badge
							asChild
							size="sm"
							className="text-[13px] font-medium text-inherit"
						>
							<span>{agentName}</span>
						</Badge>{" "}
						{`· ${title}`}
					</ToolCall.Label>
					<ToolCall.Status />
					<ToolCall.Chevron />
				</ToolCall.HeaderButton>
			</ToolCall.HeaderLayout>
			<ToolCall.Content>
				{waiting ? (
					<ToolCall.Scrollable
						className="mt-2 mb-4 rounded-lg border border-solid border-border [&_[data-streamdown=table-wrapper]]:border-0 [&_[data-streamdown=table-wrapper]]:bg-transparent [&_[data-streamdown=table-wrapper]]:p-0"
						viewportClassName="max-h-96"
						ariaLabel="Agent activity"
					>
						<div className="p-3">
							<div
								className="flex flex-col gap-2"
								role="log"
								aria-label="Agent activity"
							>
								{blocks.length > 0 ? (
									blocks.map((block, index) =>
										block.type === "tool" ? (
											<ToolComponent
												key={block.tool.id}
												organizationId={organizationId}
												name={block.tool.name}
												status={block.tool.status}
												isError={block.tool.isError}
												args={block.tool.args}
												result={block.tool.result}
												subagentTitles={subagentTitles}
												subagentVariants={subagentVariants}
												mcpServers={mcpServers}
												shellToolDisplayMode={shellToolDisplayMode}
												codeDiffDisplayMode={codeDiffDisplayMode}
											/>
										) : block.type === "thinking" ? (
											<ReasoningDisclosure
												key={`thinking-${index}`}
												id={`${session}-thinking-${index}`}
												text={block.text}
												isStreaming={
													status === "running" && index === blocks.length - 1
												}
												thinkingDisplayMode={
													preferences.data?.thinking_display_mode
												}
											/>
										) : (
											<ResponseBlock
												key={`response-${index}`}
												text={block.text}
												isStreaming={status === "running"}
												streamKey={session}
											/>
										),
									)
								) : preview ? (
									<ResponseBlock
										text={preview}
										isStreaming={status === "running"}
										streamKey={session}
									/>
								) : (
									<p className="m-0 text-xs text-content-secondary">
										{status === "running"
											? "Waiting for agent activity..."
											: "No agent activity returned."}
									</p>
								)}
							</div>
							{output?.timed_out === true && (
								<p className="mb-0 text-xs text-content-secondary">
									Wait timed out. The agent is still running.
								</p>
							)}
							{status === "completed" && (
								<p className="mb-0 mt-3 flex items-start gap-1.5 text-[11px] leading-4 text-content-secondary">
									<InfoIcon aria-hidden className="mt-0.5 size-3 shrink-0" />
									<span>
										The root agent sees the final text reply, not the full
										transcript.
									</span>
								</p>
							)}
						</div>
					</ToolCall.Scrollable>
				) : prompt ? (
					<AgentContentCard>
						<Response>{prompt}</Response>
					</AgentContentCard>
				) : null}
				{error && (
					<p role="alert" className="text-xs text-content-destructive">
						{error}
					</p>
				)}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
