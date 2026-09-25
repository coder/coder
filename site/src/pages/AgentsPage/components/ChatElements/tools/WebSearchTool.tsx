import type { FC } from "react";
import type { SourceLink } from "../../ChatConversation/types";
import { ToolCall } from "./ToolCall";
import { asString, parseArgs, parseStringList, type ToolStatus } from "./utils";
import { SourcePill } from "./WebSearchSources";

type WebSearchAction =
	| { type: "search"; queries: string[] }
	| { type: "open_page"; url: string }
	| { type: "find_in_page"; url: string; pattern: string };

/**
 * Reads the action from provider-executed web_search args. OpenAI sends
 * {type, queries | url | pattern}, Anthropic sends {query}, and older
 * OpenAI rows persisted {} or {queries} without a type.
 */
export const getWebSearchAction = (args: unknown): WebSearchAction => {
	const parsedArgs = parseArgs(args);
	const url = asString(parsedArgs?.url).trim();
	const type = asString(parsedArgs?.type);
	if (type === "open_page" && url) {
		return { type, url };
	}
	if (type === "find_in_page" && url) {
		return { type, url, pattern: asString(parsedArgs?.pattern).trim() };
	}
	const queries = parseStringList(parsedArgs?.queries) ?? [
		asString(parsedArgs?.query).trim(),
	];
	return { type: "search", queries: [...new Set(queries.filter(Boolean))] };
};

type WebSearchState = "running" | "unfinished" | "failed" | "finished";

/**
 * Both providers end a search with its tool-result, so a call without one
 * is still running while the message streams and did not finish after.
 */
export const getWebSearchState = ({
	status,
	result,
	isError,
}: {
	status: ToolStatus;
	result: unknown;
	isError: boolean;
}): WebSearchState => {
	if (isError) {
		return "failed";
	}
	if (status === "running") {
		return "running";
	}
	return result === undefined ? "unfinished" : "finished";
};

export const getWebSearchLabel = (
	action: WebSearchAction,
	state: WebSearchState,
): string => {
	let verb = { running: "Searching", finished: "Searched", base: "search" };
	let object: string;
	switch (action.type) {
		case "search":
			object =
				action.queries.length > 0
					? `for ${action.queries.join(", ")}`
					: "the web";
			break;
		case "open_page":
			verb = { running: "Opening", finished: "Opened", base: "open" };
			object = action.url;
			break;
		case "find_in_page":
			object = action.pattern
				? `${action.url} for ${action.pattern}`
				: action.url;
			break;
	}
	switch (state) {
		case "running":
			return `${verb.running} ${object}`;
		case "finished":
			return `${verb.finished} ${object}`;
		case "failed":
			return `Failed to ${verb.base} ${object}`;
		case "unfinished":
			return `Did not finish ${verb.running.toLowerCase()} ${object}`;
	}
};

type WebSearchToolProps = {
	action: WebSearchAction;
	state: WebSearchState;
	/** Pages this search returned, which are not answer citations. */
	foundPages: readonly SourceLink[];
	errorMessage?: string;
};

export const WebSearchTool: FC<WebSearchToolProps> = ({
	action,
	state,
	foundPages,
	errorMessage,
}) => {
	// The header truncates, so queries are also listed in full.
	const queries = action.type === "search" ? action.queries : [];
	let status: ToolStatus = "completed";
	if (state === "running") {
		status = "running";
	} else if (state === "failed") {
		status = "error";
	}

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={state === "failed"}
			errorMessage={errorMessage || "Web search failed"}
			hasContent={queries.length > 0 || foundPages.length > 0}
		>
			<ToolCall.Header
				iconName="web_search"
				label={getWebSearchLabel(action, state)}
				secondaryLabel={
					foundPages.length > 0 ? (
						<span className="shrink-0 text-[13px] text-content-secondary/60">
							{foundPages.length === 1
								? "1 page"
								: `${foundPages.length} pages`}
						</span>
					) : undefined
				}
			/>
			<ToolCall.Content>
				{queries.length > 0 && (
					<ul className="mt-1.5 space-y-1 pl-6 text-[13px] text-content-secondary">
						{queries.map((query) => (
							<li key={query}>{query}</li>
						))}
					</ul>
				)}
				{foundPages.length > 0 && (
					<div className="mt-1.5 flex flex-wrap items-center gap-1.5">
						{foundPages.map((page) => (
							<SourcePill key={page.url} source={page} />
						))}
					</div>
				)}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
