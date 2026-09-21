import { cn } from "cn";
import { ArrowRightIcon, BotIcon } from "lucide-react";
import type { ComponentType, ReactNode } from "react";
import { useQuery } from "react-query";
import { Link, useParams } from "react-router";
import { acpSession, acpSessionPath } from "#/api/queries/acp";
import type { ACPEntry } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { useACPSession } from "../../ACP/useACPSession";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";
import { asRecord, asString, parseArgs, type ToolStatus } from "./utils";

export const acpChatPath = (parent: string, agent: string, session: string) =>
	`/agents/${encodeURIComponent(parent)}/acp/${encodeURIComponent(agent)}/${encodeURIComponent(session)}`;

type NestedToolComponent = ComponentType<{
	name: string;
	status: ToolStatus;
	args: unknown;
	result: unknown;
	isError: boolean;
}>;

function AgentOutput({
	entries,
	ToolComponent,
}: {
	entries: readonly ACPEntry[];
	ToolComponent: NestedToolComponent;
}) {
	return (
		<div className="flex flex-col gap-2">
			{entries.map((entry) =>
				entry.kind === "tool" ? (
					<ToolComponent
						key={entry.id}
						name={entry.title || "Tool"}
						status={
							entry.status === "failed"
								? "error"
								: entry.status === "completed"
									? "completed"
									: "running"
						}
						args={entry.input}
						result={entry.text || entry.output}
						isError={entry.status === "failed"}
					/>
				) : (
					<Response key={entry.id}>{entry.text}</Response>
				),
			)}
		</div>
	);
}

function LivePreview({
	path,
	ToolComponent,
}: {
	path: string;
	ToolComponent: NestedToolComponent;
}) {
	const query = useACPSession(path);
	if (query.data === null) return <p>Session expired</p>;
	if (query.error) return <p>Workspace agent unavailable. Reconnecting...</p>;
	const entries = query.data?.entries ?? [];
	const turnEntries = entries
		.slice(entries.findLastIndex((entry) => entry.role === "user") + 1)
		.filter((entry) => entry.role === "assistant");
	return (
		<div role="log" aria-label="Live agent activity">
			{!query.connected && <p>Connecting to agent...</p>}
			<AgentOutput entries={turnEntries} ToolComponent={ToolComponent} />
		</div>
	);
}

function parseEntries(value: unknown): ACPEntry[] {
	if (!Array.isArray(value)) return [];
	return value.flatMap((item) => {
		const entry = asRecord(item);
		if (
			!entry ||
			typeof entry.id !== "string" ||
			entry.role !== "assistant" ||
			typeof entry.kind !== "string"
		)
			return [];
		return [
			{
				id: entry.id,
				role: entry.role,
				kind: entry.kind,
				text: asString(entry.text),
				title: asString(entry.title),
				status: asString(entry.status),
				input: entry.input,
				output: entry.output,
			},
		];
	});
}

function AgentContentCard({
	children,
	className,
}: {
	children: ReactNode;
	className?: string;
}) {
	return (
		<div
			className={cn(
				"mt-2 rounded-lg border border-solid border-border-default",
				className,
			)}
		>
			<div className="p-3">{children}</div>
		</div>
	);
}

function SessionCard({
	ToolComponent,
	name,
	status,
	args,
	result,
	isError,
}: {
	name: string;
	status: ToolStatus;
	args: unknown;
	result: unknown;
	isError: boolean;
	ToolComponent: NestedToolComponent;
}) {
	const { agentId = "" } = useParams();
	const input = parseArgs(args);
	const output = parseArgs(result);
	const session = asString(output?.session_id) || asString(input?.session_id);
	const agent =
		asString(output?.workspace_agent_id) || asString(input?.workspace_agent_id);
	const parent = asString(output?.parent_chat_id) || agentId;
	const path =
		session && agent && parent ? acpSessionPath(parent, agent, session) : "";
	const { data: sessionDetails } = useQuery({
		...acpSession(path),
		enabled: status === "running" && Boolean(path),
		select: (snapshot) =>
			snapshot ? { agent: snapshot.agent, title: snapshot.title } : null,
	});
	const title =
		asString(output?.title) ||
		asString(input?.title) ||
		sessionDetails?.title ||
		asString(output?.agent) ||
		asString(input?.agent) ||
		"ACP agent";
	const verbs: Record<string, [string, string]> = {
		acp_spawn_agent: ["Spawning", "Spawned"],
		acp_wait_agent: ["Waiting for", "Waited for"],
		acp_message_agent: ["Messaging", "Messaged"],
		acp_interrupt_agent: ["Interrupting", "Interrupted"],
		acp_list_agents: ["Listing", "Listed"],
	};
	const action = verbs[name]?.[status === "running" ? 0 : 1] ?? "ACP agent";
	const adapter =
		asString(output?.agent) || asString(input?.agent) || sessionDetails?.agent;
	const agentName =
		adapter === "claude_code"
			? "Claude Code"
			: adapter === "codex"
				? "Codex"
				: "ACP agent";
	const report = asString(output?.report);
	const entries = parseEntries(output?.entries);
	const error = asString(output?.error) || (isError ? asString(result) : "");
	return (
		<ToolCall.Root
			status={status}
			isError={isError || Boolean(error)}
			errorMessage={error}
			defaultExpanded
		>
			<ToolCall.HeaderLayout>
				<ToolCall.HeaderButton>
					<BotIcon className="size-4 shrink-0" />
					<ToolCall.Label>
						{action}{" "}
						<Badge
							asChild
							size="xs"
							className="text-[13px] font-medium text-inherit"
						>
							<span>{agentName}</span>
						</Badge>{" "}
						{`· ${title}`}
					</ToolCall.Label>
					<ToolCall.Status />
					<ToolCall.Chevron />
				</ToolCall.HeaderButton>
				{path && (
					<ToolCall.HeaderActions>
						<Button asChild variant="subtle" size="icon" className="size-6">
							<Link
								to={acpChatPath(parent, agent, session)}
								aria-label={`Open ${title}`}
							>
								<ArrowRightIcon className="size-4" />
							</Link>
						</Button>
					</ToolCall.HeaderActions>
				)}
			</ToolCall.HeaderLayout>
			<ToolCall.Content>
				{name === "acp_wait_agent" ? (
					((status === "running" && path) || entries.length > 0 || report) && (
						<AgentContentCard className="mb-4 max-h-96 overflow-y-auto">
							{status === "running" && path ? (
								<LivePreview path={path} ToolComponent={ToolComponent} />
							) : entries.length > 0 ? (
								<AgentOutput entries={entries} ToolComponent={ToolComponent} />
							) : (
								<Response>{report}</Response>
							)}
						</AgentContentCard>
					)
				) : name === "acp_spawn_agent" || name === "acp_message_agent" ? (
					<AgentContentCard>
						<Response>
							{asString(input?.prompt) || asString(input?.message)}
						</Response>
					</AgentContentCard>
				) : (
					<Response>
						{asString(input?.prompt) || asString(input?.message)}
					</Response>
				)}
			</ToolCall.Content>
		</ToolCall.Root>
	);
}

export function ACPTool(props: {
	ToolComponent: NestedToolComponent;
	name: string;
	status: ToolStatus;
	args: unknown;
	result: unknown;
	isError: boolean;
}) {
	if (props.name === "acp_list_agents") {
		const output = parseArgs(props.result);
		const agents = output?.agents;
		return (
			<div className="space-y-2">
				<p className="text-xs text-content-secondary">
					{props.status === "running" ? "Listing ACP agents..." : "ACP agents"}
				</p>
				{Array.isArray(agents) &&
					agents.map((agent) => {
						const value = asRecord(agent);
						return (
							<SessionCard
								key={asString(value?.session_id)}
								{...props}
								result={agent}
							/>
						);
					})}
				{Array.isArray(agents) && agents.length === 0 && (
					<p>No ACP agents in this chat.</p>
				)}
				{props.isError && <p role="alert">{asString(props.result)}</p>}
			</div>
		);
	}
	return <SessionCard {...props} />;
}
