import Editor, { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import { useEffect, useId, useRef } from "react";
import { MONOSPACE_FONT_FAMILY } from "#/theme/constants";
import { useTheme } from "#/theme/context";

loader.config({ monaco });

export type MonacoEditorProps = {
	value?: string;
	path?: string;
	onChange?: (value: string) => void;
};

// Monaco exposes the keybinding service only as a private field, so we describe
// the shape we use instead of reaching for `any`.
type StandaloneKeybindingService = {
	addDynamicKeybinding(
		commandId: string,
		keybinding: number,
		handler: () => void,
	): void;
};

type EditorWithKeybindingService = monaco.editor.IStandaloneCodeEditor & {
	readonly _standaloneKeybindingService: StandaloneKeybindingService;
};

export const MonacoEditor: React.FC<MonacoEditorProps> = ({
	onChange,
	value,
	path,
}) => {
	const theme = useTheme();
	const id = useId();
	const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
	const modelDirectory = `/template-editor/${id}/`;
	const modelPath = monaco.Uri.from({
		scheme: "inmemory",
		path: `${modelDirectory}${path ?? "untitled"}`,
	}).toString();

	// monaco-react caches models globally and only disposes the active model.
	// Keep inactive files for undo/view state, but release this session on exit.
	useEffect(
		() => () => {
			editorRef.current = null;
			for (const model of monaco.editor.getModels()) {
				if (
					model.uri.scheme === "inmemory" &&
					model.uri.path.startsWith(modelDirectory)
				)
					model.dispose();
			}
		},
		[modelDirectory],
	);

	useEffect(() => {
		document.fonts.ready
			.then(() => {
				// Ensures that all text is measured properly.
				// If this isn't done, there can be weird selection issues.
				monaco.editor.remeasureFonts();
			})
			.catch(() => {
				// Not a biggie!
			});

		monaco.editor.defineTheme("min", theme.monaco);
	}, [theme]);

	return (
		<Editor
			value={value}
			theme="vs-dark"
			options={{
				automaticLayout: true,
				tabFocusMode: true,
				fontFamily: MONOSPACE_FONT_FAMILY,
				fontSize: 14,
				wordWrap: "on",
				padding: {
					top: 16,
					bottom: 16,
				},
			}}
			path={modelPath}
			onChange={(newValue) => {
				// A previous path's subscription can outlive the model switch.
				if (
					onChange &&
					newValue !== undefined &&
					editorRef.current?.getModel()?.uri.toString() === modelPath
				) {
					onChange(newValue);
				}
			}}
			onMount={(editor) => {
				editorRef.current = editor;
				// This jank allows for Ctrl + Enter to work outside the editor.
				// We use this keybind to trigger a build.
				(
					editor as EditorWithKeybindingService
				)._standaloneKeybindingService.addDynamicKeybinding(
					"-editor.action.insertLineAfter",
					monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter,
					() => {},
				);

				editor.updateOptions({
					theme: "min",
				});
			}}
		/>
	);
};
