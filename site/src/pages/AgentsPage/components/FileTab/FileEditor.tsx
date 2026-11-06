import Editor, { loader } from "@monaco-editor/react";
import { cn } from "cn";
import * as monaco from "monaco-editor";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import { useCoderTheme } from "#/components/SyntaxHighlighter/coderTheme";
import { MONOSPACE_FONT_FAMILY } from "#/theme/constants";
import { useTheme } from "#/theme/context";
import "#/utils/monacoEnvironment";
import "./FileEditor.css";
import {
	getFileViewState,
	saveFileViewState,
} from "../../utils/mainTabStorage";
import type { FileReveal } from "../MainTabs/useMainTabs";
import { createSemanticDiagnosticsSuppressor } from "./semanticDiagnostics";

loader.config({ monaco });

const suppressSemanticDiagnostics = createSemanticDiagnosticsSuppressor(
	monaco.typescript.typescriptDefaults,
);

// Cursor moves and scrolling arrive many times a second.
const viewStateSaveDelayMs = 250;

const applyReveal = (
	editor: monaco.editor.IStandaloneCodeEditor,
	reveal: FileReveal,
) => {
	if (reveal.line !== undefined) {
		editor.setPosition({ lineNumber: reveal.line, column: 1 });
		editor.revealLineInCenter(reveal.line);
	}
	if (reveal.focus) {
		editor.focus();
	}
};

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
	/** Called when the language, the cursor or the selection changes. */
	onStatusChange?: (status: FileEditorStatus) => void;
};

export type FileEditorStatus = {
	/** The language's display name, such as "Go" or "Plain Text". */
	language: string;
	/** 1-based cursor line. */
	line: number;
	/** 1-based cursor column. */
	column: number;
	/** Characters in all selections, 0 without one. */
	selectedCharacters: number;
};

const getStatus = (
	editor: monaco.editor.IStandaloneCodeEditor,
): FileEditorStatus | undefined => {
	const model = editor.getModel();
	const position = editor.getPosition();
	if (!model || !position) {
		return undefined;
	}
	const languageId = model.getLanguageId();
	const language =
		monaco.languages
			.getLanguages()
			.find((candidate) => candidate.id === languageId)?.aliases?.[0] ??
		languageId;
	const selectedCharacters = (editor.getSelections() ?? []).reduce(
		(total, selection) => total + model.getValueLengthInRange(selection),
		0,
	);
	return {
		language,
		line: position.lineNumber,
		column: position.column,
		selectedCharacters,
	};
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
	onStatusChange,
}) => {
	const theme = useTheme();
	const coderTheme = useCoderTheme();
	const [editor, setEditor] = useState<monaco.editor.IStandaloneCodeEditor>();
	const save = useEffectEvent(onSave);
	const revealed = useEffectEvent(onRevealed);
	const reportStatus = useEffectEvent(
		(target: monaco.editor.IStandaloneCodeEditor) => {
			const status = getStatus(target);
			if (status) {
				onStatusChange?.(status);
			}
		},
	);
	// The mount applies the first reveal before Monaco paints, so the effect
	// below must not apply it again.
	const appliedRevealRef = useRef<FileReveal>(undefined);

	useEffect(() => {
		// Text measured before the monospace font loads misplaces selections.
		void document.fonts.ready.then(() => monaco.editor.remeasureFonts());
	}, []);

	useEffect(() => suppressSemanticDiagnostics(), []);

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
		reportStatus(editor);
		const listeners = [
			editor.onDidChangeCursorSelection(() => reportStatus(editor)),
			editor.onDidChangeModelLanguage(() => reportStatus(editor)),
		];
		return () => {
			for (const listener of listeners) {
				listener.dispose();
			}
		};
	}, [editor]);

	useEffect(() => {
		if (!editor) {
			return;
		}
		const saveViewState = () => {
			const viewState = editor.saveViewState();
			if (viewState) {
				saveFileViewState(chatId, path, viewState);
			}
		};
		let timer: number | undefined;
		const scheduleSave = () => {
			window.clearTimeout(timer);
			timer = window.setTimeout(saveViewState, viewStateSaveDelayMs);
		};
		const cursorListener = editor.onDidChangeCursorPosition(scheduleSave);
		const scrollListener = editor.onDidScrollChange(scheduleSave);
		return () => {
			window.clearTimeout(timer);
			cursorListener.dispose();
			scrollListener.dispose();
			saveViewState();
		};
	}, [editor, chatId, path]);

	useEffect(() => {
		if (!editor || !reveal || appliedRevealRef.current === reveal) {
			return;
		}
		appliedRevealRef.current = reveal;
		applyReveal(editor, reveal);
		revealed();
	}, [editor, reveal]);

	if (coderTheme.isLoading) {
		return null;
	}

	return (
		<div
			data-pixel="ignore"
			className="agents-file-editor relative min-h-0 flex-1"
			style={{ backgroundColor: theme.monaco.colors["editor.background"] }}
		>
			<div
				// Monaco paints the top of the file before onMount can restore the
				// view state or reveal a line, so it stays transparent until then.
				className={cn("absolute inset-0", !editor && "opacity-0")}
			>
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
						if (reveal) {
							appliedRevealRef.current = reveal;
							applyReveal(mountedEditor, reveal);
							onRevealed();
						}
						setEditor(mountedEditor);
					}}
				/>
			</div>
		</div>
	);
};

export default FileEditor;
