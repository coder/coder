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
import { clampContent } from "#/utils/text";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { ClientsBadge } from "../ClientsBadge";
import { NetworkCallBadges } from "../NetworkCallBadges";
import { ProvidersBadge } from "../ProvidersBadge";
import { TokenBadges } from "../TokenBadges";

const PROMPT_PREVIEW_MAX_CHARS = 500;

type ListSessionsRowProps = {
	session: AIBridgeSession;
	onClick?: () => void;
};

export const ListSessionsRow: React.FC<ListSessionsRowProps> = ({
	session,
	onClick,
}) => {
	const [firstName, ...otherNames] = (
		session.initiator.name ?? session.initiator.username
	)
		.trim()
		.split(/\s+/);
	const promptPreview = clampContent(
		session.last_prompt ?? "",
		PROMPT_PREVIEW_MAX_CHARS,
	);

	return (
		<TableRow
			hover
			className="cursor-pointer"
			onClick={() => {
				onClick?.();
			}}
		>
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
							<div className="font-bold">Last prompt</div>
							<div className="wrap-anywhere">{promptPreview}</div>
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</TableCell>
			<TableCell className="w-40 max-w-40">
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
			<TableCell className="w-40 max-w-40">
				<div className="min-w-0 overflow-hidden">
					<ProvidersBadge providers={session.providers} />
				</div>
			</TableCell>
			<TableCell className="w-40 max-w-40">
				<div className="min-w-0 overflow-hidden">
					<ClientsBadge clients={[session.client ?? "Unknown"]} />
				</div>
			</TableCell>
			<TableCell className="w-32">
				<div className="flex items-center">
					<TokenBadges
						inputTokens={session.token_usage_summary.input_tokens}
						outputTokens={session.token_usage_summary.output_tokens}
					/>
				</div>
			</TableCell>
			<TableCell className="w-40">
				<NetworkCallBadges summary={session.network_calls} />
			</TableCell>
			<TableCell className="w-32">
				<Badge className="bg-surface-secondary align-end">
					{session.threads}
				</Badge>
			</TableCell>
			<TableCell className="w-36 whitespace-nowrap font-normal">
				<div className="flex items-center justify-between">
					<div className="flex flex-col">
						<span>
							{formatDateTime(
								new Date(session.last_active_at),
								DATE_FORMAT.MEDIUM_DATE,
							)}
						</span>
						<span className="text-xs text-content-secondary">
							{formatDateTime(
								new Date(session.last_active_at),
								DATE_FORMAT.TIME_12H,
							)}
						</span>
					</div>
					<ChevronRightIcon className="ml-4 size-icon-sm" />
				</div>
			</TableCell>
		</TableRow>
	);
};
