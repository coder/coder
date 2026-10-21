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
			const isOpen = current.files.some((file) => file.path === path);
			if (!isOpen) {
				return {
					files: [...current.files, { path, reveal }],
					activePath: path,
				};
			}
			if (!reveal) {
				return { ...current, activePath: path };
			}
			return {
				files: current.files.map((file) =>
					file.path === path ? { ...file, reveal } : file,
				),
				activePath: path,
			};
		});
	};

	const activate = (path: string | null) => {
		setState((current) => ({ ...current, activePath: path }));
	};

	const closeFile = (path: string) => {
		setState((current) => {
			const index = current.files.findIndex((file) => file.path === path);
			if (index < 0) {
				return current;
			}
			const files = current.files.filter((file) => file.path !== path);
			if (current.activePath !== path) {
				return { ...current, files };
			}
			const neighbor = files[Math.min(index, files.length - 1)];
			return { files, activePath: neighbor?.path ?? null };
		});
	};

	return {
		files: state.files,
		activePath: state.activePath,
		openFile,
		activate,
		closeFile,
	};
};

export type MainTabsController = ReturnType<typeof useMainTabs>;
