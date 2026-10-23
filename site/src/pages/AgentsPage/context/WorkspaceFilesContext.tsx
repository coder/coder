import { createContext, useContext } from "react";
import type {
	ChatDiffStatus,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";

export type OpenFileOptions = {
	/** 1-based line to reveal. */
	line?: number;
	/** Move keyboard focus to the file. Defaults to true. */
	focus?: boolean;
};

export type WorkspaceFilesContextValue = {
	/** The workspace agent that serves the files. */
	agentId: string;
	/** The agent's default directory, when the agent reports one. */
	defaultDirectory: string | undefined;
	/** Absolute roots of the repositories the chat touched, sorted. */
	repoRoots: readonly string[];
	/**
	 * Git watcher state keyed by repo root. An entry changes identity when the
	 * watcher reports new state for that repo, which is the signal to refetch
	 * open files under it.
	 */
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	/** Opens an absolute path as a file tab in the chat view and focuses it. */
	openFile: (absolutePath: string, options?: OpenFileOptions) => void;
	/**
	 * Shows an absolute path in the Files panel and switches the right panel
	 * to it. The tree keeps its root when the root contains the path, and is
	 * otherwise rooted at the containing repository, or at the folder itself
	 * (a file's parent) outside every repository. Ancestors expand and the
	 * row is selected and scrolled into view; a folder also expands.
	 */
	revealInFiles: (absolutePath: string, options?: RevealOptions) => void;
	/** The chat's pull requests and pushed branches, one per repository. */
	diffStatuses: readonly ChatDiffStatus[];
};

export type RevealOptions = {
	/** Whether the path is a folder. Defaults to false. */
	isDirectory?: boolean;
};

/**
 * Undefined when the workspace-files experiment is off or the chat has no
 * workspace agent. Consumers then render plain text and no file actions.
 */
export const WorkspaceFilesContext = createContext<
	WorkspaceFilesContextValue | undefined
>(undefined);

export const useWorkspaceFiles = () => useContext(WorkspaceFilesContext);
