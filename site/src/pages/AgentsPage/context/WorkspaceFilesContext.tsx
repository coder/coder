import { createContext, useContext } from "react";
import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";

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
};

/**
 * Undefined when the workspace-files experiment is off or the chat has no
 * workspace agent. Consumers then render plain text and no file actions.
 */
export const WorkspaceFilesContext = createContext<
	WorkspaceFilesContextValue | undefined
>(undefined);

export const useWorkspaceFiles = () => useContext(WorkspaceFilesContext);
