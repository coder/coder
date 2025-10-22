import type { AIBridgeInterception } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { TableCell, TableRow } from "components/Table/Table";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";

type RequestLogsRowProps = {
	interception: AIBridgeInterception;
};

export const RequestLogsRow: FC<RequestLogsRowProps> = ({ interception }) => {
	const [isOpen, setIsOpen] = useState(false);

	const hasPrompt = interception.user_prompts.length > 0;

	const tokens = interception.token_usages.reduce(
		(acc, tokenUsage) =>
			acc + tokenUsage.input_tokens + tokenUsage.output_tokens,
		0,
	);
	const toolCalls = interception.tool_usages.length;

	return (
		<>
			<TableRow
				className={cn("select-none cursor-pointer hover:bg-surface-secondary")}
				onClick={() => setIsOpen(!isOpen)}
			>
				<TableCell>
					<Button
						variant="subtle"
						size="sm"
						className={cn([
							isOpen && "text-content-primary",
							"p-0 h-auto min-w-0 align-middle",
						])}
						onClick={() => setIsOpen(!isOpen)}
					>
						{isOpen ? (
							<ChevronDownIcon size={16} />
						) : (
							<ChevronRightIcon size={16} />
						)}
						<span className="sr-only">({isOpen ? "Hide" : "Show more"})</span>
						{new Date(interception.started_at).toLocaleString()}
					</Button>
				</TableCell>
				<TableCell>
					<div css={{ display: "flex", alignItems: "center", gap: 12 }}>
						<Avatar
							fallback={interception.initiator.username}
							src={interception.initiator.avatar_url}
						/>
						<div css={{ fontWeight: 500 }}>
							{interception.initiator.username}
						</div>
					</div>
				</TableCell>
				<TableCell>
					{hasPrompt && interception.user_prompts[0].prompt}
				</TableCell>
				<TableCell>{tokens}</TableCell>
				<TableCell>{toolCalls}</TableCell>
			</TableRow>
			{isOpen && (
				<TableRow>
					<TableCell colSpan={999}>
						<pre>
							{JSON.stringify(
								{
									interception,
								},
								null,
								2,
							)}
						</pre>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};
