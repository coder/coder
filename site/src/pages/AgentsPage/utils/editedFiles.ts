import type { ChatStoreState } from "../components/ChatConversation/chatStore";
import { parseArgs } from "../components/ChatElements/tools/utils";

// Reads the paths a file-editing tool call targets without validating or
// normalising the edits themselves: the selector runs on every stream
// update, so it only does the work it needs.
function pathsFromToolCall(name: string | undefined, args: unknown): string[] {
	if (name !== "edit_files" && name !== "write_file") {
		return [];
	}
	const parsed = parseArgs(args);
	const entries = name === "edit_files" ? parsed?.files : [parsed];
	if (!Array.isArray(entries)) {
		return [];
	}
	return entries.flatMap((entry: unknown) => {
		const path =
			typeof entry === "object" && entry !== null && "path" in entry
				? entry.path
				: undefined;
		return typeof path === "string" && path !== "" ? [path] : [];
	});
}

/**
 * Files the agent has edited or written during the current turn: tool
 * calls in assistant messages after the latest user message, plus calls
 * still streaming. Returned newline-joined so the selector yields a stable
 * primitive for `useSyncExternalStore` and re-renders only on change.
 */
export function selectEditedFilesThisTurn(state: ChatStoreState): string {
	const paths = new Set<string>();
	for (let i = state.orderedMessageIDs.length - 1; i >= 0; i--) {
		const message = state.messagesByID.get(state.orderedMessageIDs[i]);
		if (!message) {
			continue;
		}
		if (message.role === "user") {
			break;
		}
		for (const part of message.content ?? []) {
			if (part.type !== "tool-call") {
				continue;
			}
			for (const path of pathsFromToolCall(part.tool_name, part.args)) {
				paths.add(path);
			}
		}
	}
	for (const call of Object.values(state.streamState?.toolCalls ?? {})) {
		for (const path of pathsFromToolCall(
			call.name,
			call.args ?? call.argsRaw,
		)) {
			paths.add(path);
		}
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
	if (!sourceLocation) {
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
