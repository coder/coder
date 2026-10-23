import { isAxiosError } from "axios";
import { useEffect, useEffectEvent, useRef } from "react";
import { focusManager, useQueryClient } from "react-query";
import { API, type WorkspaceAgentFileContents } from "#/api/api";
import { workspaceAgentFileKey } from "#/api/queries/workspaceFiles";
import type {
	ChatMessage,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import type { WorkspaceFilesContextValue } from "../../context/WorkspaceFilesContext";
import {
	type ChatStore,
	type ChatStoreState,
	selectMessagesByID,
	useChatSelector,
} from "../ChatConversation/chatStore";
import { asString } from "../ChatElements/runtimeTypeUtils";
import { parseArgs, parseEditFilesArgs } from "../ChatElements/tools/utils";

type UseFileTabRefreshOptions = {
	store: ChatStore;
	workspaceFiles: WorkspaceFilesContextValue | undefined;
	/** Absolute paths of the open file tabs. */
	paths: readonly string[];
	/** The active file tab, or null while the Chat tab is active. */
	activePath: string | null;
};

/**
 * How often the active tab checks its file for changes while the page is
 * visible, when no git watcher reports the file's repository. Each check
 * reads one byte, whose response carries the whole file's ETag. The git
 * watcher polls repositories every 5 s.
 */
export const fileCheckIntervalMs = 5_000;

/** Repo roots are sorted, so the last match is the innermost repository. */
export const findRepoRoot = (repoRoots: readonly string[], path: string) =>
	repoRoots.findLast((root) => path === root || path.startsWith(`${root}/`));

/**
 * Splits a unified diff into one section per file, each starting at its
 * `diff --git` header. Undefined when the text is not a diff, such as the
 * agent's notice for a diff too large to send.
 */
const splitDiff = (diff: string): string[] | undefined => {
	if (diff === "") {
		return [];
	}
	const sections = diff.split(/^(?=diff --git )/m);
	return sections[0]?.startsWith("diff --git ") ? sections : undefined;
};

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

const gitEscapes: Readonly<Record<string, string>> = {
	a: "\x07",
	b: "\b",
	t: "\t",
	n: "\n",
	v: "\v",
	f: "\f",
	r: "\r",
};

/**
 * The path in a string git quoted in C style. Git writes bytes past ASCII
 * as octal escapes, by default, so a path's UTF-8 bytes are decoded.
 */
const unquoteGitPath = (quoted: string) => {
	const bytes: number[] = [];
	for (const [, octal, escaped, text] of quoted.matchAll(
		/\\([0-7]{3})|\\(.)|([^\\]+)/gs,
	)) {
		if (octal) {
			bytes.push(Number.parseInt(octal, 8));
		} else {
			const chars = escaped ? (gitEscapes[escaped] ?? escaped) : text;
			bytes.push(...textEncoder.encode(chars));
		}
	}
	return textDecoder.decode(new Uint8Array(bytes));
};

// Git quotes a path with control characters, quotes, backslashes or, by
// default, bytes past ASCII, such as "b/caf\303\251.txt".
const quotedTargetPattern = /"b\/((?:[^"\\]|\\.)*)"$/;

const findDiffSection = (sections: readonly string[], relativePath: string) =>
	sections.find((section) => {
		const header = section.slice(0, section.indexOf("\n"));
		const quoted = quotedTargetPattern.exec(header);
		return quoted
			? unquoteGitPath(quoted[1]) === relativePath
			: header.endsWith(` b/${relativePath}`);
	});

/**
 * Whether `diff`, a repository's diff against HEAD, creates the file at
 * `relativePath`, which HEAD then lacks: an untracked file, a new file not
 * yet committed, or the target of an uncommitted rename or copy.
 */
export const isAddedInDiff = (diff: string, relativePath: string) => {
	const section = findDiffSection(splitDiff(diff) ?? [], relativePath);
	// Content lines start with a space, "+", "-" or "\", so only header
	// lines match.
	return (
		section !== undefined &&
		/^(?:new file mode|rename from|copy from) /m.test(section)
	);
};

const hunkHeaderPattern = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

/**
 * The line in HEAD's version of the file at `relativePath` that is `line`
 * on disk, through `diff`, the repository's diff against HEAD. Lines
 * outside the diff's hunks move by the lines the hunks before them add or
 * remove, and a file outside the diff, or whose mode alone changed, keeps
 * its lines. Undefined for a line the diff adds, which HEAD lacks, and when
 * the diff says nothing about the file's lines, such as for a binary file
 * or when `diff` is not a diff.
 */
export const getHeadLine = (
	diff: string,
	relativePath: string,
	line: number,
): number | undefined => {
	const sections = splitDiff(diff);
	if (!sections) {
		return undefined;
	}
	const section = findDiffSection(sections, relativePath);
	if (!section) {
		return line;
	}
	// The next line of each side, past the hunks so far.
	let headLine = 0;
	let diskLine = 0;
	let inHunks = false;
	for (const text of section.split("\n")) {
		const hunk = hunkHeaderPattern.exec(text);
		if (hunk) {
			const [, headStart, headCount, diskStart, diskCount] = hunk;
			// With a count of 0, the start is the line before the hunk rather
			// than its first.
			headLine = Number(headStart) + (headCount === "0" ? 1 : 0);
			diskLine = Number(diskStart) + (diskCount === "0" ? 1 : 0);
			if (line < diskLine) {
				return line + headLine - diskLine;
			}
			inHunks = true;
			continue;
		}
		// The file's headers, such as "--- a/main.go", precede its hunks.
		if (!inHunks) {
			continue;
		}
		if (text.startsWith(" ")) {
			if (diskLine === line) {
				return headLine;
			}
			headLine++;
			diskLine++;
		} else if (text.startsWith("-")) {
			headLine++;
		} else if (text.startsWith("+")) {
			if (diskLine === line) {
				return undefined;
			}
			diskLine++;
		}
	}
	if (!inHunks) {
		const isModeChange =
			/^old mode /m.test(section) &&
			!/^(?:Binary files |GIT binary patch)/m.test(section);
		return isModeChange ? line : undefined;
	}
	return line + headLine - diskLine;
};

/**
 * The paths under `root` whose content may differ between two watcher
 * states. A file's content can change only if its diff against HEAD does
 * or HEAD moves. An agent that predates head_commit reports it empty, so
 * only a branch switch shows that HEAD moved.
 */
const getChangedPaths = (
	root: string,
	paths: readonly string[],
	previous: WorkspaceAgentRepoChanges,
	next: WorkspaceAgentRepoChanges,
) => {
	const headMoved =
		Boolean(previous.head_commit && next.head_commit) &&
		previous.head_commit !== next.head_commit;
	if (previous.branch !== next.branch || headMoved) {
		return paths;
	}
	const previousSections = splitDiff(previous.unified_diff ?? "");
	const nextSections = splitDiff(next.unified_diff ?? "");
	if (!previousSections || !nextSections) {
		return paths;
	}
	return paths.filter((path) => {
		const relativePath = path.slice(root.length + 1);
		return (
			findDiffSection(previousSections, relativePath) !==
			findDiffSection(nextSections, relativePath)
		);
	});
};

const fileToolNames = new Set(["write_file", "edit_files"]);

/** The paths, as the model wrote them, that a file tool call changes. */
const getToolPaths = (name: string, args: unknown): string[] => {
	if (name === "write_file") {
		const path = asString(parseArgs(args)?.path).trim();
		return path ? [path] : [];
	}
	return parseEditFilesArgs(args).map((file) => file.path);
};

/** Relative paths match every open file that ends with them. */
const matchesToolPath = (openPath: string, toolPath: string) =>
	toolPath.startsWith("/")
		? openPath === toolPath
		: openPath.endsWith(`/${toolPath.replace(/^(~|\.)\//, "")}`);

// toolResults keeps its identity while text streams, so this selector
// rerenders only when a tool result arrives or changes.
const selectToolResults = (state: ChatStoreState) =>
	state.streamState?.toolResults ?? null;

type CompletedToolResult = { id: string; name: string };

const getDurableToolResults = (
	messages: ReadonlyMap<number, ChatMessage>,
	afterMessageID: number,
) => {
	const results: CompletedToolResult[] = [];
	for (const [messageID, message] of messages) {
		if (messageID <= afterMessageID) {
			continue;
		}
		for (const part of message.content ?? []) {
			if (part.type === "tool-result" && part.tool_call_id && part.tool_name) {
				results.push({ id: part.tool_call_id, name: part.tool_name });
			}
		}
	}
	return results;
};

/** The args of a tool call, from the stream or a durable message. */
const findToolCallArgs = (state: ChatStoreState, id: string): unknown => {
	const streamed = state.streamState?.toolCalls[id];
	if (streamed) {
		return streamed.args;
	}
	for (const message of state.messagesByID.values()) {
		for (const part of message.content ?? []) {
			if (part.type === "tool-call" && part.tool_call_id === id) {
				return part.args;
			}
		}
	}
	return undefined;
};

/**
 * Rereads open files when something may have changed them: the window
 * regains focus, the git watcher reports a new diff for the file, a
 * write_file or edit_files call in this chat completes, or a periodic check
 * finds that the active file outside every watched repository changed.
 * Only the active tab's query is enabled, so it rereads at once and the
 * others on their next activation.
 */
export const useFileTabRefresh = ({
	store,
	workspaceFiles,
	paths,
	activePath,
}: UseFileTabRefreshOptions) => {
	const queryClient = useQueryClient();
	const repositories = workspaceFiles?.repositories;
	const toolResults = useChatSelector(store, selectToolResults);
	const messages = useChatSelector(store, selectMessagesByID);

	const refresh = useEffectEvent((refreshed: readonly string[]) => {
		if (!workspaceFiles) {
			return;
		}
		for (const path of refreshed) {
			const queryKey = workspaceAgentFileKey(workspaceFiles.agentId, path);
			// A read in flight may predate the change. Invalidating restarts
			// it only once the file has data, and joins the first read, so
			// the read is canceled first.
			void queryClient
				.cancelQueries({ queryKey, exact: true })
				.then(() => queryClient.invalidateQueries({ queryKey, exact: true }));
		}
	});

	const refreshAll = useEffectEvent(() => refresh(paths));
	useEffect(
		() =>
			focusManager.subscribe((isFocused) => {
				if (isFocused) {
					refreshAll();
				}
			}),
		[],
	);

	// The last state seen per repository. It survives watcher reconnects,
	// which empty the map, so the first state after one compares with the
	// last before it. The first state ever seen is a baseline, since the
	// file reads at mount already reflect it.
	const seenRepositoriesRef = useRef(
		new Map<string, WorkspaceAgentRepoChanges>(),
	);
	const refreshChangedRepositories = useEffectEvent(
		(current: ReadonlyMap<string, WorkspaceAgentRepoChanges>) => {
			if (!workspaceFiles) {
				return;
			}
			const seen = seenRepositoriesRef.current;
			for (const [root, next] of current) {
				const previous = seen.get(root);
				seen.set(root, next);
				if (!previous || previous === next) {
					continue;
				}
				const repoPaths = paths.filter(
					(path) => findRepoRoot(workspaceFiles.repoRoots, path) === root,
				);
				refresh(getChangedPaths(root, repoPaths, previous, next));
			}
		},
	);
	useEffect(() => {
		if (repositories) {
			refreshChangedRepositories(repositories);
		}
	}, [repositories]);

	const checkFile = useEffectEvent(async (path: string) => {
		if (!workspaceFiles || document.visibilityState !== "visible") {
			return;
		}
		const queryKey = workspaceAgentFileKey(workspaceFiles.agentId, path);
		if (queryClient.getQueryState(queryKey)?.fetchStatus !== "idle") {
			return;
		}
		let etag: string | undefined;
		try {
			({ etag } = await API.readWorkspaceAgentFile(
				workspaceFiles.agentId,
				path,
				{ limit: 1 },
			));
		} catch (error) {
			// A missing file has no ETag; other failures say nothing.
			if (!isAxiosError(error) || error.response?.status !== 404) {
				return;
			}
		}
		const state =
			queryClient.getQueryState<WorkspaceAgentFileContents>(queryKey);
		if (state?.fetchStatus !== "idle") {
			return;
		}
		// A read that failed, such as for a deleted file, keeps its data.
		const readEtag = state.status === "success" ? state.data?.etag : undefined;
		if (etag !== readEtag) {
			refresh([path]);
		}
	});
	const checkedPath =
		workspaceFiles &&
		activePath !== null &&
		!findRepoRoot(workspaceFiles.repoRoots, activePath)
			? activePath
			: undefined;
	useEffect(() => {
		if (!checkedPath) {
			return;
		}
		let isChecking = false;
		const timer = window.setInterval(() => {
			if (isChecking) {
				return;
			}
			isChecking = true;
			void checkFile(checkedPath).finally(() => {
				isChecking = false;
			});
		}, fileCheckIntervalMs);
		return () => window.clearInterval(timer);
	}, [checkedPath]);

	// A result can show in the stream and again in its durable message, so
	// each tool call refreshes once.
	const handledToolCallsRef = useRef(new Set<string>());
	/**
	 * Rereads the files that the results' tool calls wrote. Baseline results
	 * are only recorded: the reads at mount already cover their writes.
	 */
	const refreshWrittenFiles = useEffectEvent(
		(results: readonly CompletedToolResult[], isBaseline: boolean) => {
			const handled = handledToolCallsRef.current;
			const state = store.getSnapshot();
			const toolPaths: string[] = [];
			for (const result of results) {
				if (!fileToolNames.has(result.name) || handled.has(result.id)) {
					continue;
				}
				handled.add(result.id);
				if (!isBaseline) {
					toolPaths.push(
						...getToolPaths(result.name, findToolCallArgs(state, result.id)),
					);
				}
			}
			refresh(
				paths.filter((path) =>
					toolPaths.some((toolPath) => matchesToolPath(path, toolPath)),
				),
			);
		},
	);

	const isStreamBaselineRef = useRef(true);
	useEffect(() => {
		const results = Object.values(toolResults ?? {}).filter(
			(result) => !result.isStreaming,
		);
		refreshWrittenFiles(results, isStreamBaselineRef.current);
		isStreamBaselineRef.current = false;
	}, [toolResults]);

	// A batch that commits a turn's messages clears its stream state in the
	// same update, so a result can reach only the durable messages. Message
	// IDs grow, so the history at its first load, and older pages loaded
	// later, stay baseline.
	const lastBaselineMessageIDRef = useRef<number>(undefined);
	useEffect(() => {
		const baselineID = lastBaselineMessageIDRef.current;
		if (baselineID !== undefined) {
			refreshWrittenFiles(getDurableToolResults(messages, baselineID), false);
			return;
		}
		if (messages.size > 0) {
			lastBaselineMessageIDRef.current = Math.max(...messages.keys());
			refreshWrittenFiles(getDurableToolResults(messages, 0), true);
		}
	}, [messages]);
};
