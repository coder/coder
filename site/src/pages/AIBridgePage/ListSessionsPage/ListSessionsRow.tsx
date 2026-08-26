import { MessageSquareTextIcon } from "lucide-react";
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
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeProviderIcon } from "#/pages/AIBridgePage/icons/AIBridgeProviderIcon";
import { NetworkCallBadges } from "../NetworkCallBadges";
import { TokenBadges } from "../TokenBadges";
import { getProviderDisplayName } from "../utils";

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
			<TableCell className="max-w-32 min-w-10 flex-1 overflow-hidden font-normal">
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="inline-flex min-w-0 max-w-full items-center">
								<span className="hidden min-w-0 max-w-full truncate xl:block">
									{session.last_prompt}
								</span>
								<MessageSquareTextIcon
									aria-label="View last prompt"
									className="block size-icon-sm text-content-secondary xl:hidden"
								/>
							</span>
						</TooltipTrigger>
						<TooltipContent className="max-w-[512px]" side="top" align="start">
							<div className="font-bold">Last prompt</div>
							<div className="line-clamp-5">{session.last_prompt}</div>
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
					{session.providers.length > 1 ? (
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<span>
										<Badge className="max-w-full">
											{session.providers.length} providers
										</Badge>
									</span>
								</TooltipTrigger>
								<TooltipContent side="top" align="start" sideOffset={8}>
									<div className="flex flex-col gap-2">
										<div className="text-content-primary text-sm font-medium">
											Providers
										</div>
										{session.providers.map((provider) => (
											<div key={provider} className="flex items-center gap-2">
												<AIBridgeProviderIcon
													provider={provider}
													className="size-icon-xs"
												/>
												<span>{getProviderDisplayName(provider)}</span>
											</div>
										))}
									</div>
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					) : session.providers.length === 1 ? (
						<Badge className="gap-1.5 max-w-full">
							<div className="flex-shrink-0 flex items-center">
								<AIBridgeProviderIcon
									provider={session.providers[0]}
									className="size-icon-xs"
								/>
							</div>
							<span className="truncate min-w-0">
								{getProviderDisplayName(session.providers[0])}
							</span>
						</Badge>
					) : null}
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
			<TableCell className="w-40">
				<NetworkCallBadges summary={session.network_calls} />
			</TableCell>
			<TableCell className="w-24 max-w-24">
				<Badge className="bg-surface-secondary align-end">
					{session.threads}
				</Badge>
			</TableCell>
		</TableRow>
	);
};
