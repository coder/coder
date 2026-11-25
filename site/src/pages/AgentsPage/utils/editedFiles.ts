import type { ChatStoreState } from "../components/ChatConversation/chatStore";
import { parseArgs } from "../components/ChatElements/tools/utils";

// Completed edit_files results report which files actually changed and
// include the caller's path, so neither encoding of the files input needs
// parsing. write_file only reports success; its path comes from the input.
function editedPaths(
	name: string | undefined,
	args: unknown,
	output: unknown,
): string[] {
	const result = parseArgs(output);
	if (result?.ok !== true) {
		return [];
	}
	if (name === "edit_files") {
		return Array.isArray(result.files)
			? result.files.flatMap((file: unknown) => {
					if (typeof file !== "object" || file === null) {
						return [];
					}
					const path = "path" in file ? file.path : undefined;
					const diff = "diff" in file ? file.diff : undefined;
					return typeof path === "string" &&
						path !== "" &&
						typeof diff === "string" &&
						diff !== ""
						? [path]
						: [];
				})
			: [];
	}
	if (name === "write_file") {
		const path = parseArgs(args)?.path;
		return typeof path === "string" && path !== "" ? [path] : [];
	}
	return [];
}

/**
 * Files the agent successfully edited or wrote during the current turn:
 * completed tool results for calls after the latest user message, including
 * results still in the stream. Returned newline-joined so the selector yields
 * a stable primitive for `useSyncExternalStore` and re-renders only on change.
 */
export function selectEditedFilesThisTurn(state: ChatStoreState): string {
	const messages = [];
	for (let i = state.orderedMessageIDs.length - 1; i >= 0; i--) {
		const message = state.messagesByID.get(state.orderedMessageIDs[i]);
		if (message?.role === "user") {
			break;
		}
		if (message) {
			messages.push(message);
		}
	}
	messages.reverse();

	const results = new Map<
		string,
		{ name?: string; output: unknown; isError?: boolean }
	>();
	for (const message of messages) {
		for (const part of message.content ?? []) {
			if (part.type === "tool-result" && part.tool_call_id) {
				results.set(part.tool_call_id, {
					name: part.tool_name,
					output: part.result,
					isError: part.is_error,
				});
			}
		}
	}

	const paths = new Set<string>();
	const collect = (
		id: string | undefined,
		name: string | undefined,
		args: unknown,
		result: { name?: string; output: unknown; isError?: boolean } | undefined,
	) => {
		if (!id || !result || result.isError || result.name !== name) {
			return;
		}
		for (const path of editedPaths(name, args, result.output)) {
			paths.add(path);
		}
	};
	for (const message of messages) {
		for (const part of message.content ?? []) {
			if (part.type === "tool-call") {
				collect(
					part.tool_call_id,
					part.tool_name,
					part.args,
					results.get(part.tool_call_id ?? ""),
				);
			}
		}
	}
	for (const call of Object.values(state.streamState?.toolCalls ?? {})) {
		const result = state.streamState?.toolResults[call.id];
		if (!result || result.isStreaming) {
			continue;
		}
		collect(call.id, call.name, call.args ?? call.argsRaw, {
			name: result.name,
			output: result.result ?? result.resultRaw,
			isError: result.isError,
		});
	}
	return [...paths].join("\n");
}

// Splits the selector's output back into paths.
export function parseEditedFiles(joined: string): string[] {
	return joined.split("\n").filter((path) => path !== "");
}

// Path segments, ignoring the scheme, drive, query, and current-directory
// markers bundlers tend to prepend or append.
function pathSegments(path: string): string[] {
	return path
		.replace(/^[a-z][a-z0-9+.-]*:\/\//i, "")
		.replace(/[?#].*$/, "")
		.split(/[\\/]+/)
		.filter((segment) => segment !== "" && segment !== ".");
}

/**
 * Whether a source location reported by the previewed app (`path:line`,
 * as React's dev build records it) points at one of the files the agent
 * edited. The app's bundler may report paths from a different root than
 * the workspace the agent edits in (a container, a relative build path),
 * so the comparison is on trailing path segments. A shared basename alone
 * is not enough: `index.tsx` is everywhere.
 */
export function sourceFileWasEdited(
	sourceLocation: string | undefined,
	editedFiles: readonly string[],
): boolean {
	// The location is looked up by a page-supplied id, so it may be anything
	// a plain object hands back, not only a string or nothing.
	if (typeof sourceLocation !== "string" || sourceLocation === "") {
		return false;
	}
	const source = pathSegments(sourceLocation.replace(/:\d+(?::\d+)?$/, ""));
	if (source.length === 0) {
		return false;
	}
	return editedFiles.some((file) => {
		const edited = pathSegments(file);
		let shared = 0;
		while (
			shared < source.length &&
			shared < edited.length &&
			source[source.length - 1 - shared] === edited[edited.length - 1 - shared]
		) {
			shared++;
		}
		const identical = shared === source.length && shared === edited.length;
		return identical || shared >= 2;
	});
}
