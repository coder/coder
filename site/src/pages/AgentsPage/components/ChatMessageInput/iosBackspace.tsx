import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { CAN_USE_BEFORE_INPUT, IS_IOS } from "@lexical/utils";
import {
	$getSelection,
	$isDecoratorNode,
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

function $containsDecoratorNode(node: LexicalNode | null): boolean {
	if (!node) return false;
	if ($isDecoratorNode(node)) return true;
	if (!$isElementNode(node)) return false;
	return node.getChildren().some($containsDecoratorNode);
}

function $backspaceHitsDecoratorNode(selection: RangeSelection): boolean {
	const { anchor } = selection;
	if (anchor.type === "element") {
		const node = anchor.getNode();
		if (anchor.offset === 0) {
			const topBlock = node.getTopLevelElement() ?? node;
			return $containsDecoratorNode(topBlock.getPreviousSibling());
		}
		return $containsDecoratorNode(node.getChildAtIndex(anchor.offset - 1));
	}

	if (anchor.offset !== 0) return false;
	const textNode = anchor.getNode();
	if ($containsDecoratorNode(textNode.getPreviousSibling())) return true;

	const topBlock = textNode.getTopLevelElement();
	if (topBlock?.getFirstDescendant() !== textNode) return false;
	return $containsDecoratorNode(topBlock.getPreviousSibling());
}

export function $shouldUseNativeBackspace(
	selection: BaseSelection | null,
): boolean {
	if (!$isRangeSelection(selection)) return false;
	if (selection.getNodes().some($containsDecoratorNode)) return false;
	return !selection.isCollapsed() || !$backspaceHitsDecoratorNode(selection);
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
			if ($shouldUseNativeBackspace(selection)) return true;
			if (!$isRangeSelection(selection)) return false;

			// WebKit native deletion can desynchronize contentEditable=false decorator nodes.
			event.preventDefault();
			return editor.dispatchCommand(DELETE_CHARACTER_COMMAND, true);
		},
		COMMAND_PRIORITY_HIGH,
	);
}

const IOSBackspacePlugin: FC = function IOSBackspacePlugin() {
	const [editor] = useLexicalComposerContext();

	useEffect(() => {
		// Lexical's default Backspace handler blocks the beforeinput event WebKit needs for delete acceleration.
		return registerIOSBackspaceCommand(editor, IS_IOS && CAN_USE_BEFORE_INPUT);
	}, [editor]);

	return null;
};

export { IOSBackspacePlugin };
