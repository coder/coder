import type { FC } from "react";
import { ToolCall } from "./ToolCall";
import { asRecord, asString, parseArgs, type ToolStatus } from "./utils";
import { SourcePill } from "./WebSearchSources";

const isHttpUrl = (value: string): boolean => {
	try {
		const { protocol } = new URL(value);
		return protocol === "http:" || protocol === "https:";
	} catch {
		return false;
	}
};

// The generated tool-call args type is a string map, so fixtures pass
// arrays JSON-encoded, as find_tools args do. Accept both forms.
const parseQueryList = (value: unknown): string[] | undefined => {
	let list = value;
	if (typeof list === "string") {
		try {
			list = JSON.parse(list);
		} catch {
			return undefined;
		}
	}
	return Array.isArray(list)
		? list.map((query) => asString(query).trim())
		: undefined;
};

/**
 * Reads a provider-executed web_search call. OpenAI Responses calls carry
 * {queries: [...]} as input once the search has finished, and the server
 * persists the consulted sources as the result, {sources?: [{url}]}, when
 * the response completes. Anthropic calls carry {query} before the search
 * runs and persist an empty result.
 */
export const getWebSearchToolData = (
	args: unknown,
	result: unknown,
): { queries: string[]; sourceUrls: string[]; searchFinished: boolean } => {
	const parsedArgs = parseArgs(args);
	const argQueries = parseQueryList(parsedArgs?.queries);
	const queries = argQueries ?? [asString(parsedArgs?.query).trim()];
	// Source URLs come from the provider and become links, so only
	// web schemes are allowed.
	const sources = asRecord(result)?.sources;
	const sourceUrls = Array.isArray(sources)
		? sources.map((source) => asString(asRecord(source)?.url)).filter(isHttpUrl)
		: [];
	return {
		queries: [...new Set(queries.filter(Boolean))],
		sourceUrls: [...new Set(sourceUrls)],
		searchFinished: argQueries !== undefined,
	};
};

type WebSearchToolProps = {
	queries: readonly string[];
	/** URLs the search consulted, which are not answer citations. */
	sourceUrls: readonly string[];
	status: ToolStatus;
	isError: boolean;
};

export const WebSearchTool: FC<WebSearchToolProps> = ({
	queries,
	sourceUrls,
	status,
	isError,
}) => {
	const target = queries.length > 0 ? `for ${queries.join(", ")}` : "the web";
	let label: string;
	if (status === "running") {
		label = `Searching ${target}`;
	} else if (isError) {
		label = `Failed to search ${target}`;
	} else {
		label = `Searched ${target}`;
	}
	// The header truncates, so several queries are also listed in full.
	const listedQueries = queries.length > 1 ? queries : [];
	const sourceCount =
		sourceUrls.length === 1 ? "1 source" : `${sourceUrls.length} sources`;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage="Web search failed"
			hasContent={listedQueries.length > 0 || sourceUrls.length > 0}
		>
			<ToolCall.Header
				iconName="web_search"
				label={label}
				secondaryLabel={
					sourceUrls.length > 0 ? (
						<span className="shrink-0 text-[13px] text-content-secondary/60">
							{sourceCount}
						</span>
					) : undefined
				}
			/>
			<ToolCall.Content>
				{listedQueries.length > 0 && (
					<ul className="mt-1.5 space-y-1 pl-6 text-[13px] text-content-secondary">
						{listedQueries.map((query) => (
							<li key={query}>{query}</li>
						))}
					</ul>
				)}
				{sourceUrls.length > 0 && (
					<div className="mt-1.5 flex flex-wrap items-center gap-1.5">
						{sourceUrls.map((url) => (
							<SourcePill key={url} source={{ url, title: "" }} />
						))}
					</div>
				)}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
