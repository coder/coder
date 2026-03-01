import {
	$applyNodeReplacement,
	TextNode,
	type DOMConversionMap,
	type DOMExportOutput,
	type EditorConfig,
	type LexicalNode,
	type NodeKey,
	type SerializedTextNode,
	type Spread,
} from "lexical";

export type SerializedFileMentionNode = Spread<
	{
		filePath: string;
		fileName: string;
	},
	SerializedTextNode
>;

export class FileMentionNode extends TextNode {
	__filePath: string;
	__fileName: string;

	static getType(): string {
		return "file-mention";
	}

	static clone(node: FileMentionNode): FileMentionNode {
		return new FileMentionNode(
			node.__filePath,
			node.__fileName,
			node.__key,
		);
	}

	constructor(filePath: string, fileName: string, key?: NodeKey) {
		super(fileName, key);
		this.__filePath = filePath;
		this.__fileName = fileName;
	}

	createDOM(config: EditorConfig): HTMLElement {
		const dom = super.createDOM(config);
		dom.className =
			"inline-flex items-center gap-1 rounded px-1.5 py-0.5 bg-content-link/15 text-content-link text-sm font-medium cursor-default mx-0.5";
		dom.dataset.filePath = this.__filePath;
		dom.setAttribute("spellcheck", "false");

		// Prepend a file icon using an SVG element.
		const icon = document.createElementNS(
			"http://www.w3.org/2000/svg",
			"svg",
		);
		icon.setAttribute("width", "14");
		icon.setAttribute("height", "14");
		icon.setAttribute("viewBox", "0 0 24 24");
		icon.setAttribute("fill", "none");
		icon.setAttribute("stroke", "currentColor");
		icon.setAttribute("stroke-width", "2");
		icon.setAttribute("stroke-linecap", "round");
		icon.setAttribute("stroke-linejoin", "round");
		icon.style.display = "inline";
		icon.style.verticalAlign = "middle";
		icon.style.flexShrink = "0";
		const path = document.createElementNS(
			"http://www.w3.org/2000/svg",
			"path",
		);
		path.setAttribute(
			"d",
			"M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z",
		);
		const polyline = document.createElementNS(
			"http://www.w3.org/2000/svg",
			"polyline",
		);
		polyline.setAttribute("points", "14 2 14 8 20 8");
		icon.appendChild(path);
		icon.appendChild(polyline);

		dom.insertBefore(icon, dom.firstChild);

		return dom;
	}

	updateDOM(): boolean {
		// Returning true forces Lexical to recreate the DOM node.
		return true;
	}

	static importJSON(
		serializedNode: SerializedFileMentionNode,
	): FileMentionNode {
		const node = $createFileMentionNode(
			serializedNode.filePath,
			serializedNode.fileName,
		);
		node.setFormat(serializedNode.format);
		node.setDetail(serializedNode.detail);
		node.setMode(serializedNode.mode);
		node.setStyle(serializedNode.style);
		return node;
	}

	exportJSON(): SerializedFileMentionNode {
		return {
			...super.exportJSON(),
			type: "file-mention",
			filePath: this.__filePath,
			fileName: this.__fileName,
		};
	}

	exportDOM(): DOMExportOutput {
		const element = document.createElement("span");
		element.textContent = this.__fileName;
		element.dataset.filePath = this.__filePath;
		return { element };
	}

	static importDOM(): DOMConversionMap | null {
		return null;
	}

	isTextEntity(): true {
		return true;
	}

	canInsertTextBefore(): boolean {
		return false;
	}

	canInsertTextAfter(): boolean {
		return false;
	}

	getFilePath(): string {
		return this.__filePath;
	}

	getFileName(): string {
		return this.__fileName;
	}
}

export function $createFileMentionNode(
	filePath: string,
	fileName: string,
): FileMentionNode {
	const node = new FileMentionNode(filePath, fileName);
	// Mark as non-editable so the mention acts as an atomic token.
	node.setMode("token");
	return $applyNodeReplacement(node);
}

export function $isFileMentionNode(
	node: LexicalNode | null | undefined,
): node is FileMentionNode {
	return node instanceof FileMentionNode;
}
