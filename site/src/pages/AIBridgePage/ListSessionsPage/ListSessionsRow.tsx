import { ChevronRightIcon } from "lucide-react";
import type { AIBridgeSession } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { TableCell, TableRow } from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { ClientsBadge } from "../ClientsBadge";
import { ModelsBadge } from "../ModelsBadge";
import { NetworkCallBadges } from "../NetworkCallBadges";
import { ProvidersBadge } from "../ProvidersBadge";
import { TokenBadges } from "../TokenBadges";
import { formatSessionCost, truncatePromptPreview } from "../utils";
import type { SessionColumnId } from "./sessionColumns";

type ListSessionsRowProps = {
	session: AIBridgeSession;
	columns: readonly SessionColumnId[];
	onClick?: () => void;
};

export const ListSessionsRow: React.FC<ListSessionsRowProps> = ({
	session,
	columns,
	onClick,
}) => {
	return (
		<TableRow
			hover
			className="cursor-pointer"
			onClick={() => {
				onClick?.();
			}}
		>
			{columns.map((id) => (
				<SessionCell key={id} column={id} session={session} />
			))}
			<TableCell>
				<ChevronRightIcon className="size-icon-sm" />
			</TableCell>
		</TableRow>
	);
};

// TODO: Replace with the session cost once the sessions API reports it.
// This is synthetic placeholder data derived from token counts, not a real
// cost estimate.
const placeholderCostMicros = (session: AIBridgeSession): number =>
	session.token_usage_summary.input_tokens * 3 +
	session.token_usage_summary.output_tokens * 15;

type SessionCellProps = {
	column: SessionColumnId;
	session: AIBridgeSession;
};

const SessionCell: React.FC<SessionCellProps> = ({ column, session }) => {
	switch (column) {
		case "initial_prompt":
			return <InitialPromptCell session={session} />;
		case "user":
			return <UserCell session={session} />;
		case "provider":
			return (
				<TableCell>
					<div className="min-w-0 overflow-hidden">
						<ProvidersBadge providers={session.providers} />
					</div>
				</TableCell>
			);
		case "client":
			return (
				<TableCell>
					<div className="min-w-0 overflow-hidden">
						<ClientsBadge clients={[session.client ?? "Unknown"]} />
					</div>
				</TableCell>
			);
		case "model":
			return (
				<TableCell>
					<div className="min-w-0 overflow-hidden">
						<ModelsBadge models={session.models} />
					</div>
				</TableCell>
			);
		case "tokens":
			return (
				<TableCell>
					<div className="flex items-center">
						<TokenBadges
							inputTokens={session.token_usage_summary.input_tokens}
							outputTokens={session.token_usage_summary.output_tokens}
						/>
					</div>
				</TableCell>
			);
		case "cost":
			return (
				<TableCell className="whitespace-nowrap font-normal">
					{formatSessionCost(placeholderCostMicros(session))}
				</TableCell>
			);
		case "network":
			return (
				<TableCell>
					<NetworkCallBadges summary={session.network_calls} />
				</TableCell>
			);
		case "threads":
			return (
				<TableCell>
					<Badge className="bg-surface-secondary align-end">
						{session.threads}
					</Badge>
				</TableCell>
			);
		case "last_active":
			return (
				<TableCell className="whitespace-nowrap font-normal">
					<div className="flex flex-col">
						<span>
							{formatDateTime(
								new Date(session.last_active_at),
								DATE_FORMAT.MEDIUM_DATE,
							)}
						</span>
						<span className="text-content-secondary">
							{formatDateTime(
								new Date(session.last_active_at),
								DATE_FORMAT.TIME_12H,
							)}
						</span>
					</div>
				</TableCell>
			);
	}
};

type SessionCellContentProps = {
	session: AIBridgeSession;
};

// TODO: Replace with the initial prompt once the sessions API reports it.
// The API should return, in order of preference:
//   1. For Coder Agents sessions, the chat summary. The session ID is the
//      root chat ID (from the X-Coder-Chat-Id header), so it joins to chats.
//   2. The first user prompt that is not a shared system prompt. A prompt is
//      shared when its opening matches prompts from five or more other
//      sessions, which catches tool templates such as sidebar summarizers.
//   3. The first user prompt.
// Until then this shows the last prompt, the only prompt the API returns.
const placeholderInitialPrompt = (session: AIBridgeSession): string =>
	session.last_prompt ?? "";

const InitialPromptCell: React.FC<SessionCellContentProps> = ({ session }) => {
	const promptPreview = truncatePromptPreview(
		placeholderInitialPrompt(session),
	);
	return (
		<TableCell className="font-normal">
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<p className="m-0 line-clamp-2 wrap-anywhere">{promptPreview}</p>
					</TooltipTrigger>
					<TooltipContent
						className="max-w-64 max-h-80 overflow-y-auto"
						side="top"
						align="start"
						// The portal still bubbles React clicks up to the row.
						onClick={(event) => event.stopPropagation()}
					>
						<div className="font-bold">Initial prompt</div>
						<div className="wrap-anywhere">{promptPreview}</div>
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		</TableCell>
	);
};

const UserCell: React.FC<SessionCellContentProps> = ({ session }) => {
	const [firstName, ...otherNames] = (
		session.initiator.name ?? session.initiator.username
	)
		.trim()
		.split(/\s+/);
	return (
		<TableCell>
			<div className="flex items-center gap-3 min-w-0">
				<Avatar
					fallback={session.initiator.username}
					src={session.initiator.avatar_url}
					size="lg"
					className="shrink-0"
				/>
				<div className="flex min-w-0 flex-1 flex-col font-normal">
					<span className="truncate">{firstName}</span>
					{otherNames.length > 0 && (
						<span className="truncate">{otherNames.join(" ")}</span>
					)}
				</div>
			</div>
		</TableCell>
	);
};
