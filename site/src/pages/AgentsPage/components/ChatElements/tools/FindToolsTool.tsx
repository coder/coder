import type { FC } from "react";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

export type FindToolsMatch = {
	name: string;
	description: string;
};

type FindToolsToolProps = {
	queries: readonly string[];
	names: readonly string[];
	matches: readonly FindToolsMatch[];
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
};

export const FindToolsTool: FC<FindToolsToolProps> = ({
	queries,
	names,
	matches,
	status,
	isError,
	errorMessage,
}) => {
	const queryLabel =
		[...queries, ...names.map((name) => `name:${name}`)].join(", ") || "tools";
	const label =
		status === "running"
			? `Searching tools: ${queryLabel}`
			: `Searched tools: ${queryLabel} -> ${matches.length} matched`;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to search tools"}
			hasContent={matches.length > 0}
		>
			<ToolCall.Header iconName="find_tools" label={label} />
			<ToolCall.Content>
				<ul className="mt-1.5 space-y-2 pl-6 text-[13px] text-content-secondary">
					{matches.map((match) => (
						<li key={match.name}>
							<div className="font-medium text-content-primary">
								{match.name}
							</div>
							{match.description ? <div>{match.description}</div> : null}
						</li>
					))}
				</ul>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
