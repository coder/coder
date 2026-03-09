import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type { MinimalUser } from "api/typesGenerated";
import type { FC } from "react";
import { formatDateTime } from "utils/time";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
import { ArrowDownIcon, ArrowUpIcon } from "lucide-react";
import {
	getProviderDisplayName,
	getProviderIconName,
	roundTokenDisplay,
} from "../utils";
import { AIBridgeProviderIcon } from "../RequestLogsPage/icons/AIBridgeProviderIcon";
import { AIBridgeClientIcon } from "../RequestLogsPage/icons/AIBridgeClientIcon";

const SeparatorRow: FC = () => (
	<tr>
		<td colSpan={2} className="py-1">
			<div className="border-t border-border" />
		</td>
	</tr>
);

export interface AISessionTableProps {
	sessionId: string;
	startTime: Date;
	endTime?: Date;
	initiator: MinimalUser;
	client: string;
	provider: string[];
	inputTokens: number;
	outputTokens: number;
	threadCount: number;
	toolCallCount: number;
}

export const AISessionTable = ({
	sessionId,
	startTime,
	endTime,
	initiator,
	provider,
	client,
	inputTokens,
	outputTokens,
	threadCount,
	toolCallCount,
}: AISessionTableProps) => {
	const durationInMs =
		endTime != null
			? new Date(endTime).getTime() - new Date(startTime).getTime()
			: undefined;

	return (
		<table className="text-sm table-fixed w-full border-collapse border-spacing-0">
			<tbody>
				<tr>
					<td className="text-content-secondary pr-4 whitespace-nowrap py-2">
						Session ID
					</td>
					<td
						className="text-content-primary font-mono truncate max-w-0 py-2"
						title={sessionId}
					>
						{sessionId}
					</td>
				</tr>
				<tr>
					<td className="text-content-secondary pr-4 whitespace-nowrap pb-2">
						Start time
					</td>
					<td
						className="text-content-primary font-mono truncate max-w-0 pb-2"
						title={formatDateTime(startTime)}
					>
						{formatDateTime(startTime)}
					</td>
				</tr>
				<tr>
					<td className="text-content-secondary pr-4 whitespace-nowrap pb-2">
						End time
					</td>
					<td className="text-content-primary font-mono truncate max-w-0 pb-2">
						{endTime ? formatDateTime(endTime) : "—"}
					</td>
				</tr>
				<SeparatorRow />
				<tr>
					<td className="text-content-secondary pr-4 whitespace-nowrap pb-2">
						Duration
					</td>
					<td
						className="text-content-primary font-mono truncate max-w-0 pb-2"
						title={durationInMs != null ? `${durationInMs} ms` : undefined}
					>
						{durationInMs != null
							? `${Math.round(durationInMs / 1000)} s`
							: "—"}
					</td>
				</tr>
				<tr>
					<td className="text-content-secondary pr-4 whitespace-nowrap pb-2">
						Initiator
					</td>
					<td className="text-content-primary pb-2">
						<div className="flex items-center gap-2">
							<Avatar
								size="sm"
								src={initiator.avatar_url}
								fallback={initiator.name}
							/>
							<span className="truncate" title={initiator.name}>
								{initiator.name}
							</span>
						</div>
					</td>
				</tr>
				<tr>
					<td className="text-content-secondary pr-4 whitespace-nowrap pb-2">
						Client
					</td>
					<td
						className="text-content-primary truncate max-w-0 pb-2 text-right"
						title={client ?? "—"}
					>
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<Badge className="gap-1.5 max-w-full">
										<div className="flex-shrink-0 flex items-center">
											<AIBridgeClientIcon
												client={client}
												className="size-icon-xs"
											/>
										</div>
										<span className="truncate min-w-0">
											{client ?? "Unknown"}
										</span>
									</Badge>
								</TooltipTrigger>
								<TooltipContent>{client}</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					</td>
				</tr>
				<tr>
					<td className="align-top text-content-secondary pr-4">Provider</td>
					<td
						className="text-content-primary truncate max-w-0 text-right"
						title={provider.join(", ")}
					>
						{provider.map((p) => (
							<div key={p}>
								<Badge key={p}>
									<div className="flex-shrink-0 flex items-center">
										<AIBridgeProviderIcon
											provider={getProviderIconName(p)}
											className="size-icon-xs"
										/>
									</div>
									<span className="truncate min-w-0">
										{getProviderDisplayName(p)}
									</span>
								</Badge>
							</div>
						))}
					</td>
				</tr>
				<SeparatorRow />
				<tr>
					<td className="text-content-secondary pr-4 whitespace-nowrap py-2">
						In/out tokens
					</td>
					<td className="text-content-primary font-mono whitespace-nowrap max-w-0 py-2">
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<Badge className="gap-0 rounded-e-none">
										<ArrowDownIcon className="size-icon-lg flex-shrink-0" />
										<span className="truncate min-w-0 w-full">
											{roundTokenDisplay(inputTokens)}
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
											{roundTokenDisplay(outputTokens)}
										</span>
									</Badge>
								</TooltipTrigger>
								<TooltipContent>{outputTokens} Output Tokens</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					</td>
				</tr>
				<tr>
					<td className="text-content-secondary pr-4 whitespace-nowrap pb-2">
						Threads
					</td>
					<td className="text-content-primary font-mono truncate max-w-0 pb-2 text-right">
						<Badge>{threadCount}</Badge>
					</td>
				</tr>
				<tr>
					<td className="text-content-secondary pr-4 whitespace-nowrap pb-2">
						Tool calls
					</td>
					<td className="text-content-primary font-mono truncate max-w-0 pb-2 text-right">
						<Badge>{toolCallCount}</Badge>
					</td>
				</tr>
			</tbody>
		</table>
	);
};
