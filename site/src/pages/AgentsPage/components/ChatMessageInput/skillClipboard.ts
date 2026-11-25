import { mergeRegister } from "@lexical/utils";
import {
	$createTextNode,
	$isElementNode,
	$parseSerializedNode,
	type LexicalEditor,
	type LexicalNode,
	type RangeSelection,
	type SerializedLexicalNode,
} from "lexical";
import { getSkillAlias, type SkillSource } from "../../utils/skillAliases";
import {
	$getSkillInvocationNodes,
	$hasSkillInvocationNode,
	isSerializedSkillInvocationNode,
	MAX_SKILL_INVOCATIONS_PER_MESSAGE,
	SkillInvocationNode,
	warnSkillLimit,
} from "./SkillInvocationNode";

/**
 * Returns the serialized nodes Lexical put on the clipboard when copying
 * from an editor with the given namespace, or null for other content.
 */
export const getClipboardNodes = (
	dataTransfer: DataTransfer | null,
	namespace: string,
): SerializedLexicalNode[] | null => {
	const payload = dataTransfer?.getData("application/x-lexical-editor");
	if (!payload) {
		return null;
	}
	try {
		const { namespace: payloadNamespace, nodes } = JSON.parse(payload);
		return payloadNamespace === namespace &&
			Array.isArray(nodes) &&
			nodes.every((node) => typeof node?.type === "string")
			? nodes
			: null;
	} catch {
		return null;
	}
};

/**
 * Pastes copied composer nodes at the selection. Returns false, changing
 * nothing, when a node cannot be parsed, so the caller pastes plain text.
 */
export const $insertClipboardNodes = (
	selection: RangeSelection,
	serializedNodes: readonly SerializedLexicalNode[],
	isSkillAvailable: (source: SkillSource, name: string) => boolean,
): boolean => {
	let nodes: LexicalNode[];
	try {
		nodes = serializedNodes.map((node) => $parseSerializedNode(node));
	} catch {
		return false;
	}
	// Chips the paste replaces must not count as already in the editor.
	selection.removeText();
	const pastedAliases = new Set<string>();
	let chipCount = $getSkillInvocationNodes().length;
	let isWithinLimit = true;
	const $resolveNode = (node: LexicalNode): LexicalNode | null => {
		if (node instanceof SkillInvocationNode) {
			const { __source: source, __name: name } = node;
			const alias = getSkillAlias(source, name);
			if (!isSkillAvailable(source, name)) {
				return $createTextNode(alias);
			}
			if (pastedAliases.has(alias) || $hasSkillInvocationNode(source, name)) {
				return null;
			}
			if (chipCount >= MAX_SKILL_INVOCATIONS_PER_MESSAGE) {
				isWithinLimit = false;
				return null;
			}
			chipCount++;
			pastedAliases.add(alias);
		} else if ($isElementNode(node)) {
			for (const child of node.getChildren()) {
				const resolved = $resolveNode(child);
				if (!resolved) {
					child.remove();
				} else if (resolved !== child) {
					child.replace(resolved);
				}
			}
		}
		return node;
	};
	selection.insertNodes(
		nodes.map((node) => $resolveNode(node)).filter((node) => node !== null),
	);
	if (!isWithinLimit) {
		warnSkillLimit();
	}
	return true;
};

const getSerializedNodeText = (node: SerializedLexicalNode): string => {
	if (isSerializedSkillInvocationNode(node)) {
		return getSkillAlias(node.source, node.name);
	}
	if (node.type === "linebreak") {
		return "\n";
	}
	if ("text" in node && typeof node.text === "string") {
		return node.text;
	}
	if ("children" in node && Array.isArray(node.children)) {
		return node.children.map(getSerializedNodeText).join("");
	}
	return "";
};

/**
 * Rewrites the plain-text form of content copied or cut from the editor
 * so skill chips, which have no text content, appear as their aliases.
 * Lexical's own handlers fill the clipboard first because they listen on
 * the root element before this listener is added.
 */
export const registerSkillAliasCopy = (
	editor: LexicalEditor,
	namespace: string,
) => {
	const handleCopy = (event: ClipboardEvent) => {
		const nodes = getClipboardNodes(event.clipboardData, namespace);
		if (!nodes) {
			return;
		}
		const isBlockList = nodes.every((node) => "children" in node);
		event.clipboardData?.setData(
			"text/plain",
			nodes.map(getSerializedNodeText).join(isBlockList ? "\n" : ""),
		);
	};
	const setListeners = (root: HTMLElement | null, add: boolean) => {
		for (const type of ["copy", "cut"] as const) {
			if (add) {
				root?.addEventListener(type, handleCopy);
			} else {
				root?.removeEventListener(type, handleCopy);
			}
		}
	};
	return mergeRegister(
		editor.registerRootListener((root, prevRoot) => {
			setListeners(prevRoot, false);
			setListeners(root, true);
		}),
		() => setListeners(editor.getRootElement(), false),
	);
};
