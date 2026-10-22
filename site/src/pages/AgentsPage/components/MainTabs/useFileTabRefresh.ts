import { useEffect, useEffectEvent, useRef } from "react";
import { focusManager, useQueryClient } from "react-query";
import { workspaceAgentFileKey } from "#/api/queries/workspaceFiles";
import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";
import type { WorkspaceFilesContextValue } from "../../context/WorkspaceFilesContext";
import {
	type ChatStore,
	type ChatStoreState,
	useChatSelector,
} from "../ChatConversation/chatStore";
import { asString } from "../ChatElements/runtimeTypeUtils";
import { parseArgs, parseEditFilesArgs } from "../ChatElements/tools/utils";

type UseFileTabRefreshOptions = {
	store: ChatStore;
	workspaceFiles: WorkspaceFilesContextValue | undefined;
	/** Absolute paths of the open file tabs. */
	paths: readonly string[];
};

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

const findDiffSection = (sections: readonly string[], relativePath: string) =>
	sections.find((section) => {
		const header = section.slice(0, section.indexOf("\n"));
		return header.endsWith(` b/${relativePath}`);
	});

/**
 * The paths under `root` whose part of the repository diff differs between
 * two watcher states. A file's content can change only if its diff does.
 */
const getChangedPaths = (
	root: string,
	paths: readonly string[],
	previous: WorkspaceAgentRepoChanges,
	next: WorkspaceAgentRepoChanges,
) => {
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

/**
 * Rereads open files when something may have changed them: the window
 * regains focus, the git watcher reports a new diff for the file, or a
 * write_file or edit_files call in this chat completes. Only the active
 * tab's query is enabled, so it rereads at once and the others on their
 * next activation.
 */
export const useFileTabRefresh = ({
	store,
	workspaceFiles,
	paths,
}: UseFileTabRefreshOptions) => {
	const queryClient = useQueryClient();
	const repositories = workspaceFiles?.repositories;
	const toolResults = useChatSelector(store, selectToolResults);

	const refresh = useEffectEvent((refreshed: readonly string[]) => {
		if (!workspaceFiles) {
			return;
		}
		for (const path of refreshed) {
			void queryClient.invalidateQueries({
				queryKey: workspaceAgentFileKey(workspaceFiles.agentId, path),
				exact: true,
			});
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

	// Results already complete at mount belong to writes the mount-time
	// reads already see.
	const handledToolCallsRef = useRef<Set<string>>(undefined);
	const refreshWrittenFiles = useEffectEvent(
		(results: NonNullable<typeof toolResults>) => {
			const toolCalls = store.getSnapshot().streamState?.toolCalls ?? {};
			const isInitial = handledToolCallsRef.current === undefined;
			const handled = handledToolCallsRef.current ?? new Set();
			handledToolCallsRef.current = handled;
			const toolPaths: string[] = [];
			for (const result of Object.values(results)) {
				if (
					result.isStreaming ||
					!fileToolNames.has(result.name) ||
					handled.has(result.id)
				) {
					continue;
				}
				handled.add(result.id);
				if (!isInitial) {
					toolPaths.push(
						...getToolPaths(result.name, toolCalls[result.id]?.args),
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
	useEffect(() => {
		if (toolResults) {
			refreshWrittenFiles(toolResults);
		} else if (!handledToolCallsRef.current) {
			handledToolCallsRef.current = new Set();
		}
	}, [toolResults]);
};
