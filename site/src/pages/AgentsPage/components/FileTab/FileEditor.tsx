import Editor, { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import { useEffect, useEffectEvent, useState } from "react";
import { useCoderTheme } from "#/components/SyntaxHighlighter/coderTheme";
import { MONOSPACE_FONT_FAMILY } from "#/theme/constants";
import { useTheme } from "#/theme/context";
import "#/utils/monacoEnvironment";
import {
	getFileViewState,
	saveFileViewState,
} from "../../utils/mainTabStorage";
import type { FileReveal } from "../MainTabs/useMainTabs";

loader.config({ monaco });

// Cursor moves and scrolling arrive many times a second.
const viewStateSaveDelayMs = 250;

type FileEditorProps = {
	/** The chat whose tab shows the file; view state is saved per chat. */
	chatId: string;
	/** Absolute path. Monaco infers the language from the model URI. */
	path: string;
	value: string;
	reveal: FileReveal | undefined;
	readOnly: boolean;
	onChange: (value: string) => void;
	onSave: () => void;
	/** Called once `reveal` is applied. */
	onRevealed: () => void;
};

/** Loaded with React.lazy so the Monaco chunk loads only when a file opens. */
const FileEditor: React.FC<FileEditorProps> = ({
	chatId,
	path,
	value,
	reveal,
	readOnly,
	onChange,
	onSave,
	onRevealed,
}) => {
	const theme = useTheme();
	const coderTheme = useCoderTheme();
	const [editor, setEditor] = useState<monaco.editor.IStandaloneCodeEditor>();
	const save = useEffectEvent(onSave);
	const revealed = useEffectEvent(onRevealed);

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
		if (!editor) {
			return;
		}
		const save = () => {
			const viewState = editor.saveViewState();
			if (viewState) {
				saveFileViewState(chatId, path, viewState);
			}
		};
		let timer: number | undefined;
		const scheduleSave = () => {
			window.clearTimeout(timer);
			timer = window.setTimeout(save, viewStateSaveDelayMs);
		};
		const cursorListener = editor.onDidChangeCursorPosition(scheduleSave);
		const scrollListener = editor.onDidScrollChange(scheduleSave);
		return () => {
			window.clearTimeout(timer);
			cursorListener.dispose();
			scrollListener.dispose();
			save();
		};
	}, [editor, chatId, path]);

	useEffect(() => {
		if (!editor || !reveal) {
			return;
		}
		if (reveal.line !== undefined) {
			editor.setPosition({ lineNumber: reveal.line, column: 1 });
			editor.revealLineInCenter(reveal.line);
		}
		if (reveal.focus) {
			editor.focus();
		}
		revealed();
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
						readOnly,
						scrollBeyondLastLine: false,
						wordWrap: "on",
						padding: { top: 8, bottom: 8 },
					}}
					onChange={(nextValue) => {
						if (nextValue !== undefined) {
							onChange(nextValue);
						}
					}}
					onMount={(mountedEditor) => {
						const viewState = getFileViewState(chatId, path);
						if (viewState) {
							mountedEditor.restoreViewState(viewState);
						}
						setEditor(mountedEditor);
					}}
				/>
			</div>
		</div>
	);
};

export default FileEditor;
