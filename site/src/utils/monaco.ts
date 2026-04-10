import { loader } from "@monaco-editor/react";

let configurePromise: Promise<typeof import("monaco-editor")> | null = null;

/**
 * Lazily loads the local Monaco editor module and configures
 * @monaco-editor/react to use it instead of the CDN. This keeps the heavy
 * Monaco bundle out of the critical path for routes that never render an
 * editor.
 *
 * Returns a promise that resolves to the Monaco namespace. Safe to call
 * multiple times; only the first call triggers the dynamic import.
 */
export const ensureMonacoIsLoaded = (): Promise<
	typeof import("monaco-editor")
> => {
	if (!configurePromise) {
		configurePromise = import("monaco-editor").then((monaco) => {
			loader.config({ monaco });
			return monaco;
		});
	}

	return configurePromise;
};
