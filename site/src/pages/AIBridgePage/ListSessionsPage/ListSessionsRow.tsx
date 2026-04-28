import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
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
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeClientIcon";
import { AIBridgeProviderIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeProviderIcon";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { TokenBadges } from "../TokenBadges";
import { getProviderDisplayName, getProviderIconName } from "../utils";

type ListSessionsRowProps = {
	session: AIBridgeSession;
	onClick?: () => void;
};

export const ListSessionsRow: FC<ListSessionsRowProps> = ({
	session,
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
			<TableCell className="max-w-32 flex-1 overflow-auto font-normal">
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<p className="truncate">{session.last_prompt}</p>
						</TooltipTrigger>
						<TooltipContent className="max-w-64" side="top" align="start">
							<div className="font-bold">Last prompt</div>
							<div>{session.last_prompt}</div>
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</TableCell>
			<TableCell className="w-48 max-w-48">
				<div className="w-full min-w-0 overflow-hidden">
					<div className="flex items-center gap-3 min-w-0">
						<Avatar
							fallback={session.initiator.username}
							src={session.initiator.avatar_url}
							size="lg"
							className="flex-shrink-0"
						/>
						<div className="font-normal truncate min-w-0 flex-1 overflow-hidden">
							{session.initiator.name ?? session.initiator.username}
						</div>
					</div>
				</div>
			</TableCell>
			<TableCell className="w-40 max-w-40">
				<div className="min-w-0 overflow-hidden">
					<Badge className="gap-1.5 max-w-full">
						<div className="flex-shrink-0 flex items-center">
							<AIBridgeProviderIcon
								provider={getProviderIconName(session.providers[0])}
								className="size-icon-xs"
							/>
						</div>
						<span className="truncate min-w-0">
							{getProviderDisplayName(session.providers[0])}
						</span>
					</Badge>
				</div>
			</TableCell>
			<TableCell className="w-40 max-w-40">
				<div className="min-w-0 overflow-hidden">
					<Badge className="gap-1.5 max-w-full">
						<div className="flex-shrink-0 flex items-center">
							<AIBridgeClientIcon
								client={session.client}
								className="size-icon-xs"
							/>
						</div>
						<span className="truncate min-w-0">
							{session.client ?? "Unknown"}
						</span>
					</Badge>
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
			<TableCell className="w-32">
				<Badge className="bg-surface-secondary align-end">
					{session.threads}
				</Badge>
			</TableCell>
			<TableCell className="w-48 whitespace-nowrap font-normal">
				<div className="flex items-center justify-between">
					<span>
						{formatDateTime(
							new Date(session.last_active_at),
							DATE_FORMAT.FULL_DATETIME,
						)}
					</span>
					<ChevronRightIcon className="ml-4 size-icon-sm" />
				</div>
			</TableCell>
		</TableRow>
	);
};
