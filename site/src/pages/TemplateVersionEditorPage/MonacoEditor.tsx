import { useTheme } from "@emotion/react";
import Editor from "@monaco-editor/react";
import type * as Monaco from "monaco-editor";
import { type FC, useCallback, useEffect, useRef, useState } from "react";
import { MONOSPACE_FONT_FAMILY } from "#/theme/constants";
import { ensureMonacoIsLoaded } from "#/utils/monaco";

export interface MonacoEditorProps {
	value?: string;
	path?: string;
	onChange?: (value: string) => void;
}

export const MonacoEditor: FC<MonacoEditorProps> = ({
	onChange,
	value,
	path,
}) => {
	const theme = useTheme();
	const monacoRef = useRef<typeof Monaco | null>(null);
	const [monacoReady, setMonacoReady] = useState(false);

	const configureMonacoTheme = useCallback(
		(monacoInstance: typeof Monaco) => {
			document.fonts.ready
				.then(() => {
					// Ensures that all text is measured properly.
					// If this isn't done, there can be weird selection issues.
					monacoInstance.editor.remeasureFonts();
				})
				.catch(() => {
					// Not a biggie!
				});

			monacoInstance.editor.defineTheme("min", theme.monaco);
		},
		[theme],
	);

	useEffect(() => {
		let isMounted = true;

		void ensureMonacoIsLoaded()
			.then((monacoInstance) => {
				monacoRef.current = monacoInstance;
				if (isMounted) {
					setMonacoReady(true);
				}
			})
			.catch(() => {
				// Not a biggie!
			});

		return () => {
			isMounted = false;
		};
	}, []);

	useEffect(() => {
		if (!monacoReady || !monacoRef.current) {
			return;
		}

		configureMonacoTheme(monacoRef.current);
	}, [configureMonacoTheme, monacoReady]);

	if (!monacoReady) {
		return null;
	}

	return (
		<Editor
			value={value}
			theme="vs-dark"
			options={{
				automaticLayout: true,
				fontFamily: MONOSPACE_FONT_FAMILY,
				fontSize: 14,
				wordWrap: "on",
				padding: {
					top: 16,
					bottom: 16,
				},
			}}
			path={path}
			onChange={(newValue) => {
				if (onChange && newValue !== undefined) {
					onChange(newValue);
				}
			}}
			onMount={(editor, monacoInstance) => {
				monacoRef.current = monacoInstance;
				setMonacoReady(true);
				configureMonacoTheme(monacoInstance);

				// This jank allows for Ctrl + Enter to work outside the editor.
				// We use this keybind to trigger a build.
				// biome-ignore lint/suspicious/noExplicitAny: Private type in Monaco!
				(editor as any)._standaloneKeybindingService.addDynamicKeybinding(
					"-editor.action.insertLineAfter",
					monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyCode.Enter,
					() => {},
				);

				editor.updateOptions({
					theme: "min",
				});
			}}
		/>
	);
};
