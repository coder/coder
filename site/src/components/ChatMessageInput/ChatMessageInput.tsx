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
	$isRangeSelection,
	COMMAND_PRIORITY_HIGH,
	FORMAT_ELEMENT_COMMAND,
	FORMAT_TEXT_COMMAND,
	KEY_ENTER_COMMAND,
	type LexicalEditor,
	PASTE_COMMAND,
	type ParagraphNode,
} from "lexical";
import {
	type FC,
	memo,
	useCallback,
	useEffect,
	useImperativeHandle,
	useMemo,
	useRef,
} from "react";
import { cn } from "utils/cn";

// Blocks Cmd+B/I/U and element formatting shortcuts so the editor
// stays plain-text only.
const DisableFormattingPlugin: FC = memo(function DisableFormattingPlugin() {
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
});

// Intercepts paste events and inserts clipboard content as plain text,
// stripping any rich-text formatting.
const PasteSanitizationPlugin: FC = memo(function PasteSanitizationPlugin() {
	const [editor] = useLexicalComposerContext();

	useEffect(() => {
		return editor.registerCommand(
			PASTE_COMMAND,
			(event: ClipboardEvent | null) => {
				if (!event) return false;
				const clipboardData = event.clipboardData;
				if (!clipboardData) return false;

				const text = clipboardData.getData("text/plain");
				if (!text) return false;

				event.preventDefault();

				editor.update(() => {
					const selection = $getSelection();
					if ($isRangeSelection(selection)) {
						selection.insertText(text);
					} else {
						const root = $getRoot();
						const lastChild = root.getLastChild();
						if (lastChild) {
							if (lastChild.getType() === "paragraph") {
								const paragraph = lastChild as ParagraphNode;
								const textNode = $createTextNode(text);
								paragraph.append(textNode);
								textNode.selectEnd();
							} else {
								const textNode = $createTextNode(text);
								lastChild.insertAfter(textNode);
								textNode.selectEnd();
							}
						} else {
							const paragraph = $createParagraphNode();
							const textNode = $createTextNode(text);
							paragraph.append(textNode);
							root.append(paragraph);
							textNode.selectEnd();
						}
					}
				});

				return true;
			},
			COMMAND_PRIORITY_HIGH,
		);
	}, [editor]);

	return null;
});

// Handles Enter key behavior: plain Enter submits via the onEnter
// callback, Shift+Enter inserts a newline.
const EnterKeyPlugin: FC<{ onEnter?: () => void }> = memo(
	function EnterKeyPlugin({ onEnter }) {
		const [editor] = useLexicalComposerContext();

		useEffect(() => {
			return editor.registerCommand(
				KEY_ENTER_COMMAND,
				(event: KeyboardEvent | null) => {
					if (event?.shiftKey) {
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
	},
);

// Fires the onChange callback with the editor's plain-text content
// on every update.
const ContentChangePlugin: FC<{
	onChange?: (content: string) => void;
}> = memo(function ContentChangePlugin({ onChange }) {
	const [editor] = useLexicalComposerContext();

	useEffect(() => {
		if (!onChange) return;

		return editor.registerUpdateListener(({ editorState }) => {
			editorState.read(() => {
				const root = $getRoot();
				const content = root.getTextContent();
				onChange(content);
			});
		});
	}, [editor, onChange]);

	return null;
});

// Seeds the editor with an initial value on first mount.
const ValueSyncPlugin: FC<{ initialValue?: string }> = memo(
	function ValueSyncPlugin({ initialValue }) {
		const [editor] = useLexicalComposerContext();
		const hasInitialized = useRef(false);

		useEffect(() => {
			if (!hasInitialized.current && initialValue !== undefined) {
				hasInitialized.current = true;

				if (initialValue === "") {
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
			}
		}, [editor, initialValue]);

		return null;
	},
);

// Exposes the LexicalEditor instance to the parent via a callback
// so it can be stored in a ref for imperative access.
const InsertTextPlugin: FC<{
	onEditorReady: (editor: LexicalEditor) => void;
}> = memo(function InsertTextPlugin({ onEditorReady }) {
	const [editor] = useLexicalComposerContext();

	useEffect(() => {
		onEditorReady(editor);
	}, [editor, onEditorReady]);

	return null;
});

export interface ChatMessageInputRef {
	insertText: (text: string) => void;
	clear: () => void;
	focus: () => void;
	getValue: () => string;
}

interface ChatMessageInputProps
	extends Omit<React.ComponentProps<"div">, "onChange" | "role" | "ref"> {
	placeholder?: string;
	initialValue?: string;
	onChange?: (content: string) => void;
	rows?: number;
	onEnter?: () => void;
	disabled?: boolean;
	autoFocus?: boolean;
	"aria-label"?: string;
}

const ChatMessageInput = memo(
	({
		className,
		placeholder,
		initialValue,
		onChange,
		rows,
		onEnter,
		disabled,
		autoFocus,
		"aria-label": ariaLabel,
		ref,
		...props
	}: ChatMessageInputProps & { ref?: React.Ref<ChatMessageInputRef> }) => {
		const initialConfig = useMemo(
			() => ({
				namespace: "ChatMessageInput",
				theme: {
					paragraph: "m-0",
				},
				onError: (error: Error) => console.error("Lexical error:", error),
				nodes: [],
			}),
			[],
		);
		const style = useMemo(
			() => ({
				minHeight: rows ? `${rows * 1.5}rem` : undefined,
			}),
			[rows],
		);

		const editorRef = useRef<LexicalEditor | null>(null);

		const handleEditorReady = useCallback((editor: LexicalEditor) => {
			editorRef.current = editor;
		}, []);

		const handleContentChange = useCallback(
			(content: string) => {
				onChange?.(content);
			},
			[onChange],
		);

		useImperativeHandle(
			ref,
			() => ({
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
							const root = $getRoot();
							const lastChild = root.getLastChild();
							if (lastChild) {
								if (lastChild.getType() === "paragraph") {
									const paragraph = lastChild as ParagraphNode;
									const textNode = $createTextNode(text);
									paragraph.append(textNode);
									textNode.selectEnd();
								} else {
									const textNode = $createTextNode(text);
									lastChild.insertAfter(textNode);
									textNode.selectEnd();
								}
							} else {
								const paragraph = $createParagraphNode();
								const textNode = $createTextNode(text);
								paragraph.append(textNode);
								root.append(paragraph);
								textNode.selectEnd();
							}
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
					if (!editor) return "";
					let content = "";
					editor.getEditorState().read(() => {
						content = $getRoot().getTextContent();
					});
					return content;
				},
			}),
			[],
		);

		return (
			<LexicalComposer initialConfig={initialConfig} key={initialValue}>
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
								className="outline-none w-full whitespace-pre-wrap [&_p]:leading-normal [&_p:first-child]:mt-0 [&_p:last-child]:mb-0"
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
					<PasteSanitizationPlugin />
					<EnterKeyPlugin onEnter={disabled ? undefined : onEnter} />
					<ContentChangePlugin onChange={handleContentChange} />
					<ValueSyncPlugin initialValue={initialValue} />
					<InsertTextPlugin onEditorReady={handleEditorReady} />
					{autoFocus && <AutoFocusPlugin />}
				</div>
			</LexicalComposer>
		);
	},
);
ChatMessageInput.displayName = "ChatMessageInput";

export { ChatMessageInput };
