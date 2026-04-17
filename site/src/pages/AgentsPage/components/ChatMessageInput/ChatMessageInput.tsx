import { AutoFocusPlugin } from "@lexical/react/LexicalAutoFocusPlugin";
import { LexicalComposer } from "@lexical/react/LexicalComposer";
import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { ContentEditable } from "@lexical/react/LexicalContentEditable";
import { LexicalErrorBoundary } from "@lexical/react/LexicalErrorBoundary";
import { HistoryPlugin } from "@lexical/react/LexicalHistoryPlugin";
import { RichTextPlugin } from "@lexical/react/LexicalRichTextPlugin";
import { mergeRegister } from "@lexical/utils";
import {
	$createParagraphNode,
	$createTextNode,
	$getRoot,
	$getSelection,
	$insertNodes,
	$isParagraphNode,
	$isRangeSelection,
	COMMAND_PRIORITY_HIGH,
	FORMAT_ELEMENT_COMMAND,
	FORMAT_TEXT_COMMAND,
	KEY_DOWN_COMMAND,
	KEY_ENTER_COMMAND,
	type LexicalEditor,
	PASTE_COMMAND,
} from "lexical";
import {
	type FC,
	useEffect,
	useImperativeHandle,
	useLayoutEffect,
	useRef,
} from "react";
import { cn } from "#/utils/cn";
import { isMobileViewport } from "#/utils/mobile";
import {
	$createFileReferenceNode,
	FileReferenceNode,
} from "./FileReferenceNode";
import {
	createPasteFile,
	getPasteDataTransfer,
	getPastedPlainText,
	isLargePaste,
	type PasteCommandEvent,
} from "./pasteHelpers";

// Blocks Cmd+B/I/U and element formatting shortcuts so the editor
// stays plain-text only.
const DisableFormattingPlugin: FC = function DisableFormattingPlugin() {
	const [editor] = useLexicalComposerContext();

	useEffect(() => {
		return mergeRegister(
			editor.registerCommand(
				FORMAT_TEXT_COMMAND,
				() => true,
				COMMAND_PRIORITY_HIGH,
			),
			editor.registerCommand(
				FORMAT_ELEMENT_COMMAND,
				() => true,
				COMMAND_PRIORITY_HIGH,
			),
		);
	}, [editor]);

	return null;
};

function insertPlainTextIntoEditor(editor: LexicalEditor, text: string) {
	editor.update(() => {
		const selection = $getSelection();
		if ($isRangeSelection(selection)) {
			selection.insertText(text);
			return;
		}
		const root = $getRoot();
		const lastChild = root.getLastChild();

		if (lastChild && $isParagraphNode(lastChild)) {
			const textNode = $createTextNode(text);
			lastChild.append(textNode);
			textNode.selectEnd();
		} else if (lastChild) {
			const textNode = $createTextNode(text);
			lastChild.insertAfter(textNode);
			textNode.selectEnd();
		} else {
			const paragraph = $createParagraphNode();
			const textNode = $createTextNode(text);
			paragraph.append(textNode);
			root.append(paragraph);
			textNode.selectEnd();
		}
	});
}

// Replaces the entire editor content with the given plain text.
// Used by the imperative setValue() method.
function replacePlainTextInEditor(editor: LexicalEditor, text: string) {
	editor.update(() => {
		const root = $getRoot();
		root.clear();
		const paragraph = $createParagraphNode();
		root.append(paragraph);
		if (!text) {
			paragraph.select();
			return;
		}
		paragraph.select();
		const selection = $getSelection();
		if ($isRangeSelection(selection)) {
			selection.insertText(text);
			return;
		}
		const textNode = $createTextNode(text);
		paragraph.append(textNode);
		textNode.selectEnd();
	});
}

// Intercepts paste events and inserts clipboard content as plain text,
// stripping any rich-text formatting. Image files and large pasted text
// are forwarded to the parent via the onFilePaste callback instead.
//
// Cmd/Ctrl+Shift+V ("paste and match style") is treated as an explicit
// user intent to paste inline, so the large-paste-to-attachment
// conversion is bypassed for that shortcut.
const PasteSanitizationPlugin: FC<{
	onFilePaste?: (file: File) => void;
	allowTextAttachmentPaste?: boolean;
}> = function PasteSanitizationPlugin({
	onFilePaste,
	allowTextAttachmentPaste = true,
}) {
	const [editor] = useLexicalComposerContext();
	const plainTextPasteRef = useRef(false);
	const plainTextPasteTimeoutRef = useRef<number | null>(null);

	useEffect(() => {
		const unregister = mergeRegister(
			// Detect Cmd/Ctrl+Shift+V so the PASTE_COMMAND handler
			// can bypass attachment conversion for that shortcut.
			editor.registerCommand(
				KEY_DOWN_COMMAND,
				(event: KeyboardEvent) => {
					if (
						event.shiftKey &&
						(event.metaKey || event.ctrlKey) &&
						event.key.toLowerCase() === "v"
					) {
						plainTextPasteRef.current = true;
						if (plainTextPasteTimeoutRef.current !== null) {
							window.clearTimeout(plainTextPasteTimeoutRef.current);
						}
						plainTextPasteTimeoutRef.current = window.setTimeout(() => {
							plainTextPasteRef.current = false;
							plainTextPasteTimeoutRef.current = null;
						}, 500);
					}
					return false;
				},
				COMMAND_PRIORITY_HIGH,
			),

			editor.registerCommand(
				PASTE_COMMAND,
				(event: PasteCommandEvent | null) => {
					if (!event) return false;

					const isPlainTextPaste = plainTextPasteRef.current;
					plainTextPasteRef.current = false;
					if (plainTextPasteTimeoutRef.current !== null) {
						window.clearTimeout(plainTextPasteTimeoutRef.current);
						plainTextPasteTimeoutRef.current = null;
					}
					const isNativePaste = "clipboardData" in event;
					const dataTransfer = getPasteDataTransfer(event);

					// Some browsers deliver paste as beforeinput with
					// payload on `event.data` / `dataTransfer` instead of
					// a native ClipboardEvent. Consume that payload here so
					// plain-text paste shortcuts never become a no-op.
					if (!isNativePaste) {
						const text = getPastedPlainText(event, dataTransfer);
						if (!text) {
							return false;
						}
						if (
							!isPlainTextPaste &&
							allowTextAttachmentPaste &&
							onFilePaste &&
							isLargePaste(text)
						) {
							event.preventDefault();
							onFilePaste(createPasteFile(text));
							return true;
						}
						event.preventDefault();
						insertPlainTextIntoEditor(editor, text);
						return true;
					}
					// Native paste event (ClipboardEvent).

					// Check for image files in the clipboard (e.g.
					// pasted screenshots). Forward them to the parent
					// via callback instead of inserting text.
					if (onFilePaste && dataTransfer?.files.length) {
						const images = Array.from(dataTransfer.files).filter((f) =>
							f.type.startsWith("image/"),
						);
						if (images.length > 0) {
							event.preventDefault();
							for (const file of images) {
								onFilePaste(file);
							}
							return true;
						}
					}

					const text = getPastedPlainText(event, dataTransfer);
					if (!text) return false;

					// Convert large pastes to file attachments, but
					// only for normal Cmd+V. Cmd+Shift+V is the
					// user’s explicit "paste inline" escape hatch.
					if (
						!isPlainTextPaste &&
						allowTextAttachmentPaste &&
						onFilePaste &&
						isLargePaste(text)
					) {
						event.preventDefault();
						onFilePaste(createPasteFile(text));
						return true;
					}

					// Small paste (or Cmd+Shift+V): insert as plain text.
					event.preventDefault();
					insertPlainTextIntoEditor(editor, text);

					return true;
				},
				COMMAND_PRIORITY_HIGH,
			),
		);

		return () => {
			if (plainTextPasteTimeoutRef.current !== null) {
				window.clearTimeout(plainTextPasteTimeoutRef.current);
				plainTextPasteTimeoutRef.current = null;
			}
			unregister();
		};
	}, [allowTextAttachmentPaste, editor, onFilePaste]);

	return null;
};

// Handles Enter key behavior: plain Enter submits via the onEnter
// callback, Shift+Enter inserts a newline. On mobile viewports, Enter
// always inserts a newline; users submit via the send button because
// Shift+Enter is cumbersome on touch keyboards (CODAGT-210).
const EnterKeyPlugin: FC<{ onEnter?: () => void }> = function EnterKeyPlugin({
	onEnter,
}) {
	const [editor] = useLexicalComposerContext();

	useEffect(() => {
		return editor.registerCommand(
			KEY_ENTER_COMMAND,
			(event: KeyboardEvent | null) => {
				if (event?.shiftKey || isMobileViewport()) {
					return false;
				}
				if (onEnter) {
					event?.preventDefault();
					onEnter();
				}
				return true;
			},
			COMMAND_PRIORITY_HIGH,
		);
	}, [editor, onEnter]);

	return null;
};

// Fires the onChange callback with the editor's plain-text content
// on every update.
const ContentChangePlugin: FC<{
	onChange?: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
}> = function ContentChangePlugin({ onChange }) {
	const [editor] = useLexicalComposerContext();

	useEffect(() => {
		if (!onChange) return;

		return editor.registerUpdateListener(({ editorState }) => {
			editorState.read(() => {
				const root = $getRoot();
				const content = root.getTextContent();
				let hasRefs = false;

				for (const child of root.getChildren()) {
					if (!$isParagraphNode(child)) continue;

					for (const node of child.getChildren()) {
						if (node instanceof FileReferenceNode) {
							hasRefs = true;
							break;
						}
					}

					if (hasRefs) break;
				}
				const serialized = JSON.stringify(editorState.toJSON());
				onChange(content, serialized, hasRefs);
			});
		});
	}, [editor, onChange]);

	return null;
};

// Seeds the editor with an initial value on first mount. When
// initialEditorState is provided (a serialized Lexical JSON string),
// it restores the full editor state including file-reference chips.
// Falls back to plain-text seeding via initialValue.
const ValueSyncPlugin: FC<{
	initialValue?: string;
	initialEditorState?: string;
}> = function ValueSyncPlugin({ initialValue, initialEditorState }) {
	const [editor] = useLexicalComposerContext();
	const hasInitialized = useRef(false);

	useEffect(() => {
		if (hasInitialized.current) {
			return;
		}
		hasInitialized.current = true;

		// Prefer restoring the full serialized editor state
		// (preserves file-reference chips and node positions).
		if (initialEditorState) {
			try {
				const parsed = editor.parseEditorState(initialEditorState);
				editor.setEditorState(parsed);
				return;
			} catch {
				// Malformed state — fall through to plain-text path.
			}
		}

		if (initialValue === undefined || initialValue === "") {
			return;
		}

		editor.update(() => {
			const root = $getRoot();
			root.clear();
			const paragraph = $createParagraphNode();
			const textNode = $createTextNode(initialValue);
			paragraph.append(textNode);
			root.append(paragraph);
		});
	}, [editor, initialValue, initialEditorState]);

	return null;
};

// Exposes the LexicalEditor instance to the parent via a callback
// so it can be stored in a ref for imperative access.
const InsertTextPlugin: FC<{
	onEditorReady: (editor: LexicalEditor) => void;
}> = function InsertTextPlugin({ onEditorReady }) {
	const [editor] = useLexicalComposerContext();

	useEffect(() => {
		onEditorReady(editor);
	}, [editor, onEditorReady]);

	return null;
};

/**
 * Structured data for a file reference extracted from the editor.
 */
interface FileReferenceData {
	readonly fileName: string;
	readonly startLine: number;
	readonly endLine: number;
	readonly content: string;
}

/**
 * A content part extracted from the Lexical editor in document order.
 * Either a text segment or a file-reference chip.
 */
type EditorContentPart =
	| { readonly type: "text"; readonly text: string }
	| {
			readonly type: "file-reference";
			readonly reference: FileReferenceData;
	  };

// Mutable variant used internally while building the parts
// array so we can append to the last text segment without
// casting away readonly.
type MutableTextPart = { type: "text"; text: string };
type MutableFileRefPart = {
	type: "file-reference";
	reference: FileReferenceData;
};
type MutableContentPart = MutableTextPart | MutableFileRefPart;

export interface ChatMessageInputRef {
	setValue: (text: string) => void;
	insertText: (text: string) => void;
	clear: () => void;
	focus: () => void;
	getValue: () => string;
	/**
	 * Insert a file reference chip in a single Lexical update
	 * (atomic for undo/redo).
	 */
	addFileReference: (ref: FileReferenceData) => void;
	/**
	 * Walk the Lexical tree in document order and return interleaved
	 * text / file-reference parts. Adjacent text nodes within the same
	 * paragraph are merged, and paragraphs are separated by newlines.
	 */
	getContentParts: () => EditorContentPart[];
}

interface ChatMessageInputProps
	extends Omit<React.ComponentProps<"div">, "onChange" | "role" | "ref"> {
	placeholder?: string;
	initialValue?: string;
	/**
	 * Serialized Lexical editor state JSON. When provided, the editor
	 * restores the full state (including file-reference chips) instead
	 * of using initialValue as plain text.
	 */
	initialEditorState?: string;
	onChange?: (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => void;
	/** Monotonic counter to force editor remount. */
	remountKey?: number;
	rows?: number;
	onEnter?: () => void;
	onFilePaste?: (file: File) => void;
	allowTextAttachmentPaste?: boolean;
	disabled?: boolean;
	autoFocus?: boolean;
	"aria-label"?: string;
}

// Keeps the Lexical editor's editable state in sync with the
// disabled prop so that the underlying contentEditable element
// becomes truly non-interactive when the input is disabled.
const EditableStatePlugin: FC<{ disabled: boolean }> =
	function EditableStatePlugin({ disabled }) {
		const [editor] = useLexicalComposerContext();

		useLayoutEffect(() => {
			editor.setEditable(!disabled);
		}, [editor, disabled]);

		return null;
	};

const ChatMessageInput = ({
	className,
	placeholder,
	initialValue,
	initialEditorState,
	onChange,
	remountKey,
	rows,
	onEnter,
	onFilePaste,
	allowTextAttachmentPaste,
	disabled,
	autoFocus,
	"aria-label": ariaLabel,
	ref,
	...props
}: ChatMessageInputProps & { ref?: React.Ref<ChatMessageInputRef> }) => {
	const initialConfig = {
		namespace: "ChatMessageInput",
		theme: {
			paragraph: "m-0",
			inlineDecorator: "mx-1",
		},
		onError: (error: Error) => console.error("Lexical error:", error),
		nodes: [FileReferenceNode],
		editable: !disabled,
	};
	const style = {
		minHeight: rows ? `${rows * 1.5}rem` : undefined,
	};

	const editorRef = useRef<LexicalEditor | null>(null);
	// Tracks the last known text content so getValue() can return
	// a useful value before the Lexical editor hydrates.
	const lastKnownValueRef = useRef(initialValue ?? "");
	// Queues a setValue call made before the editor ref is ready.
	const pendingReplacementRef = useRef<string | null>(null);

	const handleEditorReady = (editor: LexicalEditor) => {
		editorRef.current = editor;
		// Flush any queued setValue that arrived before the editor
		// was ready (e.g. useLayoutEffect in a parent).
		const pending = pendingReplacementRef.current;
		if (pending !== null) {
			pendingReplacementRef.current = null;
			replacePlainTextInEditor(editor, pending);
		}
	};

	useImperativeHandle(
		ref,
		() => ({
			setValue: (text: string) => {
				lastKnownValueRef.current = text;
				const editor = editorRef.current;
				if (!editor) {
					pendingReplacementRef.current = text;
					return;
				}
				pendingReplacementRef.current = null;
				replacePlainTextInEditor(editor, text);
			},
			insertText: (text: string) => {
				const editor = editorRef.current;
				if (!editor) return;

				editor.update(() => {
					const selection = $getSelection();
					if ($isRangeSelection(selection)) {
						const textNode = $createTextNode(text);
						$insertNodes([textNode]);
						textNode.selectEnd();
					} else {
						insertPlainTextIntoEditor(editor, text);
					}
				});
			},
			clear: () => {
				const editor = editorRef.current;
				if (!editor) return;

				editor.update(() => {
					const root = $getRoot();
					root.clear();
					const paragraph = $createParagraphNode();
					root.append(paragraph);
					paragraph.select();
				});
			},
			focus: () => {
				const editor = editorRef.current;
				if (!editor) return;
				editor.focus(() => {
					editor.update(() => {
						const root = $getRoot();
						const last = root.getLastChild();
						if (!last) {
							const paragraph = $createParagraphNode();
							root.append(paragraph);
							paragraph.select();
							return;
						}
						last.selectEnd();
					});
				});
			},
			getValue: () => {
				const editor = editorRef.current;
				if (!editor) {
					return lastKnownValueRef.current;
				}
				let content = "";
				editor.getEditorState().read(() => {
					content = $getRoot().getTextContent();
				});
				return content;
			},
			addFileReference: (ref: FileReferenceData) => {
				const editor = editorRef.current;
				if (!editor) return;

				editor.update(() => {
					const root = $getRoot();
					const firstChild = root.getFirstChild();
					const paragraph = $isParagraphNode(firstChild)
						? firstChild
						: $createParagraphNode();

					if (!$isParagraphNode(firstChild)) {
						root.append(paragraph);
					}

					const chipNode = $createFileReferenceNode(
						ref.fileName,
						ref.startLine,
						ref.endLine,
						ref.content,
					);
					paragraph.append(chipNode);
					chipNode.selectNext();
				});
			},
			getContentParts: () => {
				const editor = editorRef.current;
				if (!editor) return [];

				const parts: MutableContentPart[] = [];

				const appendText = (str: string) => {
					const last = parts[parts.length - 1];
					if (last?.type === "text") {
						last.text += str;
					} else {
						parts.push({ type: "text", text: str });
					}
				};

				editor.getEditorState().read(() => {
					const paragraphs = $getRoot().getChildren();

					for (let i = 0; i < paragraphs.length; i++) {
						const para = paragraphs[i];
						if (!$isParagraphNode(para)) continue;

						// Separate paragraphs with a newline in the
						// preceding text part, just like getTextContent().
						if (i > 0) {
							appendText("\n");
						}

						for (const node of para.getChildren()) {
							if (node instanceof FileReferenceNode) {
								parts.push({
									type: "file-reference",
									reference: {
										fileName: node.__fileName,
										startLine: node.__startLine,
										endLine: node.__endLine,
										content: node.__content,
									},
								});
							} else {
								const t = node.getTextContent();
								if (t) appendText(t);
							}
						}
					}
				});

				return parts as EditorContentPart[];
			},
		}),
		[],
	);

	return (
		<LexicalComposer initialConfig={initialConfig} key={remountKey}>
			<div
				className={cn(
					"grid w-full rounded-md bg-transparent text-base placeholder:text-content-secondary focus-visible:outline-none whitespace-pre-wrap break-words [&>*]:col-start-1 [&>*]:row-start-1",
					disabled && "cursor-not-allowed opacity-50",
					className,
				)}
				style={style}
				{...props}
			>
				<RichTextPlugin
					contentEditable={
						<ContentEditable
							className="outline-none w-full whitespace-pre-wrap overflow-y-auto max-h-[50vh] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent] [&_p]:leading-normal [&_p:first-child]:mt-0 [&_p:last-child]:mb-0 py-px"
							data-testid="chat-message-input"
							style={{ minHeight: "inherit" }}
							aria-label={ariaLabel}
							aria-disabled={disabled}
						/>
					}
					placeholder={
						<div className="pointer-events-none text-content-secondary [&_p]:leading-normal">
							{placeholder}
						</div>
					}
					ErrorBoundary={LexicalErrorBoundary}
				/>
				<HistoryPlugin />
				<DisableFormattingPlugin />
				<PasteSanitizationPlugin
					onFilePaste={onFilePaste}
					allowTextAttachmentPaste={allowTextAttachmentPaste}
				/>
				<EnterKeyPlugin onEnter={disabled ? undefined : onEnter} />
				<ContentChangePlugin onChange={onChange} />
				<ValueSyncPlugin
					initialValue={initialValue}
					initialEditorState={initialEditorState}
				/>
				<InsertTextPlugin onEditorReady={handleEditorReady} />
				<EditableStatePlugin disabled={Boolean(disabled)} />
				{autoFocus && <AutoFocusPlugin />}
			</div>
		</LexicalComposer>
	);
};
ChatMessageInput.displayName = "ChatMessageInput";

export { ChatMessageInput };
