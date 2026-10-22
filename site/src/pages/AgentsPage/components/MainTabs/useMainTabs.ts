import { useEffect, useState } from "react";
import type { OpenFileOptions } from "../../context/WorkspaceFilesContext";
import {
	getPersistedMainTabs,
	savePersistedMainTabs,
} from "../../utils/mainTabStorage";

/**
 * A request to reveal a line. Each request is a new object, so revealing the
 * same line twice still scrolls the editor.
 */
export type FileReveal = { line: number };

type MainFileTab = {
	/** Absolute path, also the tab's identity. */
	path: string;
	reveal?: FileReveal;
};

type MainTabsState = {
	files: MainFileTab[];
	/** Null when the Chat tab is active. */
	activePath: string | null;
	/**
	 * Counts activations, including of the tab that is already active, so the
	 * tab strip scrolls the active tab into view after each one.
	 */
	activation: number;
};

/**
 * Open file tabs and the active tab of the chat view, persisted per chat.
 * The page view owns this state because `openFile` is reachable from the
 * whole page (chat, right panel, quick open), not only the tab strip.
 */
export const useMainTabs = (chatId: string) => {
	const [state, setState] = useState<MainTabsState>(() => {
		const persisted = getPersistedMainTabs(chatId);
		return {
			files: persisted.paths.map((path) => ({ path })),
			activePath: persisted.activePath,
			activation: 0,
		};
	});

	useEffect(() => {
		savePersistedMainTabs(chatId, {
			paths: state.files.map((file) => file.path),
			activePath: state.activePath,
		});
	}, [chatId, state]);

	const openFile = (path: string, options?: OpenFileOptions) => {
		const reveal =
			options?.line === undefined ? undefined : { line: options.line };
		setState((current) => {
			const activation = current.activation + 1;
			const isOpen = current.files.some((file) => file.path === path);
			if (!isOpen) {
				return {
					files: [...current.files, { path, reveal }],
					activePath: path,
					activation,
				};
			}
			if (!reveal) {
				return { ...current, activePath: path, activation };
			}
			return {
				files: current.files.map((file) =>
					file.path === path ? { ...file, reveal } : file,
				),
				activePath: path,
				activation,
			};
		});
	};

	const activate = (path: string | null) => {
		setState((current) => ({
			...current,
			activePath: path,
			activation: current.activation + 1,
		}));
	};

	const closeFiles = (paths: readonly string[]) => {
		setState((current) => {
			const closing = new Set(paths);
			const files = current.files.filter((file) => !closing.has(file.path));
			if (files.length === current.files.length) {
				return current;
			}
			const activation = current.activation + 1;
			if (current.activePath === null || !closing.has(current.activePath)) {
				return { ...current, files, activation };
			}
			// The nearest remaining tab to the right of the closed active tab,
			// else the nearest to its left.
			const activeIndex = current.files.findIndex(
				(file) => file.path === current.activePath,
			);
			const isRemaining = (file: MainFileTab) => !closing.has(file.path);
			const neighbor =
				current.files.slice(activeIndex + 1).find(isRemaining) ??
				current.files.slice(0, activeIndex).findLast(isRemaining);
			return { files, activePath: neighbor?.path ?? null, activation };
		});
	};

	return {
		files: state.files,
		activePath: state.activePath,
		activation: state.activation,
		openFile,
		activate,
		closeFiles,
	};
};

export type MainTabsController = ReturnType<typeof useMainTabs>;
