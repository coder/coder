import {
	$createParagraphNode,
	$createTextNode,
	$getRoot,
	$getSelection,
	$isRangeSelection,
	createEditor,
} from "lexical";
import { describe, expect, it } from "vitest";
import {
	$createWorkspacePathNode,
	WorkspacePathNode,
} from "./WorkspacePathNode";

describe("WorkspacePathNode", () => {
	// jsdom lacks Selection.modify, so Backspace keystrokes cannot drive
	// Lexical here; this runs the deletion the Backspace command performs.
	it("is deleted as a unit by a backward delete", () => {
		const editor = createEditor({
			namespace: "workspace-path-test",
			nodes: [WorkspacePathNode],
			onError: (error) => {
				throw error;
			},
		});
		let text = "";

		editor.update(
			() => {
				const paragraph = $createParagraphNode();
				paragraph.append(
					$createTextNode("see "),
					$createWorkspacePathNode("/home/coder/a/src", true),
				);
				$getRoot().append(paragraph);
				paragraph.selectEnd();
				const selection = $getSelection();
				if ($isRangeSelection(selection)) {
					selection.deleteCharacter(true);
				}
				text = $getRoot().getTextContent();
			},
			{ discrete: true },
		);

		expect(text).toBe("see ");
	});

	it("exports its sent text to HTML", () => {
		const editor = createEditor({
			namespace: "workspace-path-test",
			nodes: [WorkspacePathNode],
			onError: (error) => {
				throw error;
			},
		});
		let html: string | undefined;

		editor.update(
			() => {
				const { element } = $createWorkspacePathNode(
					"/repo/My File.ts",
					false,
				).exportDOM(editor);
				html = element instanceof HTMLElement ? element.outerHTML : undefined;
			},
			{ discrete: true },
		);

		expect(html).toBe("<span>`/repo/My File.ts`</span>");
	});
});
