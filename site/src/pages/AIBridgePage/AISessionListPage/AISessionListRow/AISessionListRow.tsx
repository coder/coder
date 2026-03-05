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
import { ArrowDownIcon, ArrowUpIcon, ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
import { DATE_FORMAT, formatDateTime } from "utils/time";
import { AIBridgeClientIcon } from "../../RequestLogsPage/icons/AIBridgeClientIcon";
import { AIBridgeProviderIcon } from "../../RequestLogsPage/icons/AIBridgeProviderIcon";

function getProviderDisplayName(provider: AIBridgeInterception["provider"]) {
	switch (provider) {
		case "anthropic":
			return "Anthropic";
		case "openai":
			return "OpenAI";
		case "copilot":
			return "Github";
		default:
			return "Unknown";
	}
}

// the current AIBridgeProviderIcon uses the claude icon for the anthropic
// provider. while it's still in use in the RequestLogsPage, we need to hack
// around it here, but when we delete that page, we can just swap the icon
function getProviderIconName(provider: AIBridgeInterception["provider"]) {
	if (provider === "anthropic") {
		return "anthropic-neue";
	}
	return provider;
}

type AISessionListRowProps = {
	interception: AIBridgeInterception;
};

export const AISessionListRow: FC<AISessionListRowProps> = ({
	interception,
}) => {
	const inputTokens = interception.token_usages.reduce(
		(acc, tokenUsage) => acc + tokenUsage.input_tokens,
		0,
	);
	const outputTokens = interception.token_usages.reduce(
		(acc, tokenUsage) => acc + tokenUsage.output_tokens,
		0,
	);

	// TODO this should be the thread count
	const threadCount = interception.tool_usages.length;

	// TODO v2 this will come from the API as a separate field, but for now we
	// want to display the last user prompt in the interception
	const lastPrompt = interception.user_prompts.at(-1)?.prompt ?? "N/A";

	return (
		<TableRow hover>
			<TableCell className="max-w-32 flex-1 overflow-auto">
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<p className="truncate">{lastPrompt}</p>
						</TooltipTrigger>
						<TooltipContent className="max-w-64">{lastPrompt}</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</TableCell>
			<TableCell className="w-48 max-w-48">
				<div className="w-full min-w-0 overflow-hidden">
					<div className="flex items-center gap-3 min-w-0">
						<Avatar
							fallback={interception.initiator.username}
							src={interception.initiator.avatar_url}
							size="lg"
							className="flex-shrink-0"
						/>
						<div className="font-medium truncate min-w-0 flex-1 overflow-hidden">
							{interception.initiator.name ?? interception.initiator.username}
						</div>
					</div>
				</div>
			</TableCell>
			<TableCell className="w-40 max-w-40">
				<div className="min-w-0 overflow-hidden">
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Badge className="gap-1.5 max-w-full">
									<div className="flex-shrink-0 flex items-center">
										<AIBridgeProviderIcon
											provider={getProviderIconName(interception.provider)}
											className="size-icon-xs"
										/>
									</div>
									<span className="truncate min-w-0">
										{getProviderDisplayName(interception.provider)}
									</span>
								</Badge>
							</TooltipTrigger>
							<TooltipContent>
								{getProviderDisplayName(interception.provider)}
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</div>
			</TableCell>
			<TableCell className="w-40 max-w-40">
				<div className="min-w-0 overflow-hidden">
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Badge className="gap-1.5 max-w-full">
									<div className="flex-shrink-0 flex items-center">
										<AIBridgeClientIcon
											client={interception.client}
											className="size-icon-xs"
										/>
									</div>
									<span className="truncate min-w-0">
										{interception.client ?? "Unknown"}
									</span>
								</Badge>
							</TooltipTrigger>
							<TooltipContent>{interception.client}</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</div>
			</TableCell>
			<TableCell className="w-32">
				<div className="flex items-center">
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Badge className="gap-0 rounded-e-none">
									<ArrowDownIcon className="size-icon-lg flex-shrink-0" />
									<span className="truncate min-w-0 w-full">{inputTokens}</span>
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
			<TableCell className="w-32">
				<Badge className="bg-surface-secondary align-end">{threadCount}</Badge>
			</TableCell>
			<TableCell className="w-48 whitespace-nowrap">
				<div className="flex items-center justify-between">
					<span>
						{formatDateTime(
							new Date(interception.started_at),
							DATE_FORMAT.FULL_DATETIME,
						)}
					</span>
					<ChevronRightIcon className="ml-4" />
				</div>
			</TableCell>
		</TableRow>
	);
};
