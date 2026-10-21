import Editor, { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import { type FC, useEffect, useEffectEvent, useState } from "react";
import { useCoderTheme } from "#/components/SyntaxHighlighter/coderTheme";
import { MONOSPACE_FONT_FAMILY } from "#/theme/constants";
import { useTheme } from "#/theme/context";
import type { FileReveal } from "../MainTabs/useMainTabs";

loader.config({ monaco });

type FileEditorProps = {
	/** Absolute path. Monaco infers the language from the model URI. */
	path: string;
	value: string;
	reveal: FileReveal | undefined;
	onChange: (value: string) => void;
	onSave: () => void;
};

/** Loaded with React.lazy so the Monaco chunk loads only when a file opens. */
const FileEditor: FC<FileEditorProps> = ({
	path,
	value,
	reveal,
	onChange,
	onSave,
}) => {
	const theme = useTheme();
	const coderTheme = useCoderTheme();
	const [editor, setEditor] = useState<monaco.editor.IStandaloneCodeEditor>();
	const save = useEffectEvent(onSave);

	useEffect(() => {
		// Text measured before the monospace font loads misplaces selections.
		void document.fonts.ready.then(() => monaco.editor.remeasureFonts());
	}, []);

	useEffect(() => {
		if (!editor) {
			return;
		}
		const action = editor.addAction({
			id: "coder.agents.saveFile",
			label: "Save File",
			keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS],
			run: () => save(),
		});
		return () => action.dispose();
	}, [editor]);

	useEffect(() => {
		if (!editor || !reveal) {
			return;
		}
		editor.setPosition({ lineNumber: reveal.line, column: 1 });
		editor.revealLineInCenter(reveal.line);
		editor.focus();
	}, [editor, reveal]);

	if (coderTheme.isLoading) {
		return null;
	}

	return (
		<div
			data-pixel="ignore"
			className="relative min-h-0 flex-1"
			style={{ backgroundColor: theme.monaco.colors["editor.background"] }}
		>
			<div className="absolute inset-0">
				<Editor
					path={monaco.Uri.file(path).toString()}
					value={value}
					theme={coderTheme.name}
					loading={null}
					options={{
						fontFamily: MONOSPACE_FONT_FAMILY,
						fontSize: 13,
						minimap: { enabled: false },
						scrollBeyondLastLine: false,
						wordWrap: "on",
						padding: { top: 8, bottom: 8 },
					}}
					onChange={(nextValue) => {
						if (nextValue !== undefined) {
							onChange(nextValue);
						}
					}}
					onMount={(mountedEditor) => setEditor(mountedEditor)}
				/>
			</div>
		</div>
	);
};

export default FileEditor;
