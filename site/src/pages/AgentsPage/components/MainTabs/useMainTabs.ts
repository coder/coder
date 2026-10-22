import { useEffect, useState } from "react";
import type { OpenFileOptions } from "../../context/WorkspaceFilesContext";
import {
	getPersistedMainTabs,
	savePersistedMainTabs,
} from "../../utils/mainTabStorage";

/**
 * A request to reveal a line, move focus into the file, or both, applied once
 * the file's content is ready. Each request is a new object, so repeating a
 * request applies it again.
 */
export type FileReveal = {
	/** 1-based line to reveal. */
	line?: number;
	focus: boolean;
};

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

const setReveal = (
	files: MainFileTab[],
	path: string,
	reveal: FileReveal | undefined,
) =>
	reveal
		? files.map((file) => (file.path === path ? { ...file, reveal } : file))
		: files;

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
		const focus = options?.focus ?? true;
		const reveal =
			options?.line === undefined && !focus
				? undefined
				: { line: options?.line, focus };
		setState((current) => {
			const isOpen = current.files.some((file) => file.path === path);
			return {
				files: isOpen
					? setReveal(current.files, path, reveal)
					: [...current.files, { path, reveal }],
				activePath: path,
				activation: current.activation + 1,
			};
		});
	};

	/**
	 * Activates a tab. With `focus`, a file tab also moves focus into the file;
	 * the Chat tab's composer lives outside this state, so callers focus it.
	 */
	const activate = (path: string | null, options?: { focus?: boolean }) => {
		setState((current) => ({
			files:
				path === null || !options?.focus
					? current.files
					: setReveal(current.files, path, { focus: true }),
			activePath: path,
			activation: current.activation + 1,
		}));
	};

	/**
	 * Closes the tabs at `paths`. When that closes the active tab, its nearest
	 * remaining neighbor becomes active and takes focus.
	 */
	const closeFiles = (paths: readonly string[]) => {
		setState((current) => {
			const closing = new Set(paths);
			const isRemaining = (file: MainFileTab) => !closing.has(file.path);
			const files = current.files.filter(isRemaining);
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
			const neighbor =
				current.files.slice(activeIndex + 1).find(isRemaining) ??
				current.files.slice(0, activeIndex).findLast(isRemaining);
			if (!neighbor) {
				return { files, activePath: null, activation };
			}
			return {
				files: setReveal(files, neighbor.path, { focus: true }),
				activePath: neighbor.path,
				activation,
			};
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
