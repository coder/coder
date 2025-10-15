import type { AIBridgeInterception } from "api/typesGenerated";
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
					<div
						css={{
							display: "flex",
							alignItems: "center",
							justifyContent: "center",
						}}
					>
						{isOpen ? (
							<ChevronDownIcon size={16} />
						) : (
							<ChevronRightIcon size={16} />
						)}
					</div>
				</TableCell>
				<TableCell>{interception.started_at}</TableCell>
				<TableCell>{interception.initiator_id}</TableCell>
				<TableCell>
					{hasPrompt && interception.user_prompts[0].prompt}
				</TableCell>
				<TableCell>{tokens}</TableCell>
				<TableCell>{toolCalls}</TableCell>
				<TableCell>Status</TableCell>
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
