import type { FC } from "react";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { cn } from "#/utils/cn";
import { formatDate } from "#/utils/time";
import { TokenBadges } from "../../TokenBadges";

interface ToolCallTableProps {
	timestamp: Date;
	serverURL: string;
	inputTokens: number;
	outputTokens: number;
	tokenUsageMetadata?: Record<string, unknown>;
	className?: string;
}

export const ToolCallTable: FC<ToolCallTableProps> = ({
	timestamp,
	serverURL,
	inputTokens,
	outputTokens,
	tokenUsageMetadata,
	className,
}) => {
	return (
		<div
			className={cn(
				className,
				"flex flex-col gap-2 text-sm text-content-secondary font-normal",
			)}
		>
			<div className="flex items-center justify-between whitespace-nowrap">
				<span className="pr-4">In / out tokens</span>
				<TokenBadges
					inputTokens={inputTokens}
					outputTokens={outputTokens}
					tokenUsageMetadata={tokenUsageMetadata}
				/>
			</div>
			<div className="flex items-center justify-between">
				<span className="pr-4">Started at</span>
				<span
					className="font-mono whitespace-nowrap truncate"
					title={formatDate(timestamp)}
				>
					{formatDate(timestamp)}
				</span>
			</div>
			{serverURL && (
				<div className="flex items-center justify-between">
					<span className="pr-4">MCP server</span>
					<span className="font-mono truncate">{serverURL}</span>
					<CopyButton text={serverURL} label="Copy MCP server URL" />
				</div>
			)}
		</div>
	);
};
