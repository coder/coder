import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { CAN_USE_BEFORE_INPUT, IS_IOS } from "@lexical/utils";
import {
	$getSelection,
	$isElementNode,
	$isRangeSelection,
	type BaseSelection,
	COMMAND_PRIORITY_HIGH,
	DELETE_CHARACTER_COMMAND,
	KEY_BACKSPACE_COMMAND,
	type LexicalEditor,
	type LexicalNode,
	type RangeSelection,
} from "lexical";
import { type FC, useEffect } from "react";
import { FileReferenceNode } from "./FileReferenceNode";

function containsFileReference(node: LexicalNode | null): boolean {
	if (!node) return false;
	if (node instanceof FileReferenceNode) return true;
	if (!$isElementNode(node)) return false;
	return node.getChildren().some(containsFileReference);
}

function backspaceHitsFileReference(selection: RangeSelection): boolean {
	const { anchor } = selection;
	if (anchor.type === "element") {
		const node = anchor.getNode();
		if (anchor.offset === 0) {
			const topBlock = node.getTopLevelElement() ?? node;
			return containsFileReference(topBlock.getPreviousSibling());
		}
		return containsFileReference(node.getChildAtIndex(anchor.offset - 1));
	}

	if (anchor.offset !== 0) return false;
	const textNode = anchor.getNode();
	if (containsFileReference(textNode.getPreviousSibling())) return true;

	const topBlock = textNode.getTopLevelElement();
	if (topBlock?.getFirstDescendant() !== textNode) return false;
	return containsFileReference(topBlock.getPreviousSibling());
}

export function shouldUseNativeIOSBackspace(
	selection: BaseSelection | null,
): boolean {
	if (!$isRangeSelection(selection)) return false;
	if (selection.getNodes().some(containsFileReference)) return false;
	return !selection.isCollapsed() || !backspaceHitsFileReference(selection);
}

export function registerIOSBackspaceCommand(
	editor: LexicalEditor,
	useNativeBackspace: boolean,
): () => void {
	return editor.registerCommand(
		KEY_BACKSPACE_COMMAND,
		(event) => {
			if (!useNativeBackspace) return false;

			const selection = $getSelection();
			if (shouldUseNativeIOSBackspace(selection)) return true;
			if (!$isRangeSelection(selection)) return false;

			// WebKit native deletion can desynchronize contentEditable=false chips.
			event.preventDefault();
			return editor.dispatchCommand(DELETE_CHARACTER_COMMAND, true);
		},
		COMMAND_PRIORITY_HIGH,
	);
}

const IOSBackspacePlugin: FC = function IOSBackspacePlugin() {
	const [editor] = useLexicalComposerContext();

	useEffect(() => {
		// Lexical's default Backspace command prevents the beforeinput
		// event that WebKit uses for keyboard state and delete acceleration.
		return registerIOSBackspaceCommand(editor, IS_IOS && CAN_USE_BEFORE_INPUT);
	}, [editor]);

	return null;
};

export { IOSBackspacePlugin };
