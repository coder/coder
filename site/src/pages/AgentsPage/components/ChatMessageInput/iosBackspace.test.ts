import {
	$createParagraphNode,
	$createRangeSelection,
	$createTextNode,
	$getRoot,
	$getSelection,
	$setSelection,
	COMMAND_PRIORITY_HIGH,
	createEditor,
	DELETE_CHARACTER_COMMAND,
	KEY_BACKSPACE_COMMAND,
} from "lexical";
import { describe, expect, it } from "vitest";
import {
	$createFileReferenceNode,
	FileReferenceNode,
} from "./FileReferenceNode";
import {
	$shouldUseNativeBackspace,
	registerIOSBackspaceCommand,
} from "./iosBackspace";

function readBackspaceDecision(setup: () => void) {
	const editor = createEditor({
		namespace: "ios-backspace-test",
		nodes: [FileReferenceNode],
		onError: (error) => {
			throw error;
		},
	});
	let result = false;

	editor.update(
		() => {
			setup();
			result = $shouldUseNativeBackspace($getSelection());
		},
		{ discrete: true },
	);

	return result;
}

describe("$shouldUseNativeBackspace", () => {
	it("allows native deletion from plain text selections", () => {
		const result = readBackspaceDecision(() => {
			const root = $getRoot();
			const paragraph = $createParagraphNode();
			const text = $createTextNode("hello");
			root.append(paragraph);
			paragraph.append(text);
			text.select(5, 5);
		});

		expect(result).toBe(true);
	});

	it("allows native deletion when a file reference precedes a mid-text caret", () => {
		const result = readBackspaceDecision(() => {
			const root = $getRoot();
			const paragraph = $createParagraphNode();
			const fileReference = $createFileReferenceNode(
				"main.go",
				1,
				1,
				"package main",
			);
			const text = $createTextNode("hello");
			root.append(paragraph);
			paragraph.append(fileReference, text);
			text.select(3, 3);
		});

		expect(result).toBe(true);
	});

	it("keeps Lexical deletion when backspace would hit a file reference", () => {
		const result = readBackspaceDecision(() => {
			const root = $getRoot();
			const paragraph = $createParagraphNode();
			const fileReference = $createFileReferenceNode(
				"main.go",
				1,
				1,
				"package main",
			);
			const text = $createTextNode("hello");
			root.append(paragraph);
			paragraph.append(fileReference, text);
			text.select(0, 0);
		});

		expect(result).toBe(false);
	});

	it("keeps Lexical deletion when backspace would hit a file reference in the previous paragraph", () => {
		const result = readBackspaceDecision(() => {
			const root = $getRoot();
			const previousParagraph = $createParagraphNode();
			const currentParagraph = $createParagraphNode();
			const fileReference = $createFileReferenceNode(
				"main.go",
				1,
				1,
				"package main",
			);
			const text = $createTextNode("hello");
			root.append(previousParagraph, currentParagraph);
			previousParagraph.append(fileReference);
			currentParagraph.append(text);
			text.select(0, 0);
		});

		expect(result).toBe(false);
	});

	it("keeps Lexical deletion when the selection contains a file reference", () => {
		const result = readBackspaceDecision(() => {
			const root = $getRoot();
			const paragraph = $createParagraphNode();
			const before = $createTextNode("before");
			const fileReference = $createFileReferenceNode(
				"main.go",
				1,
				1,
				"package main",
			);
			const after = $createTextNode("after");
			root.append(paragraph);
			paragraph.append(before, fileReference, after);

			const selection = $createRangeSelection();
			selection.anchor.set(before.getKey(), 0, "text");
			selection.focus.set(after.getKey(), 5, "text");
			$setSelection(selection);
		});

		expect(result).toBe(false);
	});
});

describe("registerIOSBackspaceCommand", () => {
	it("falls through when native backspace is disabled", () => {
		const editor = createEditor({
			namespace: "ios-backspace-command-disabled-test",
			onError: (error) => {
				throw error;
			},
		});
		let deleteCharacterDispatched = false;

		registerIOSBackspaceCommand(editor, false);
		editor.registerCommand(
			DELETE_CHARACTER_COMMAND,
			() => {
				deleteCharacterDispatched = true;
				return true;
			},
			COMMAND_PRIORITY_HIGH,
		);

		const event = new KeyboardEvent("keydown", {
			cancelable: true,
			key: "Backspace",
		});

		expect(editor.dispatchCommand(KEY_BACKSPACE_COMMAND, event)).toBe(false);
		expect(event.defaultPrevented).toBe(false);
		expect(deleteCharacterDispatched).toBe(false);
	});

	it("prevents default and dispatches Lexical deletion near file references", () => {
		const editor = createEditor({
			namespace: "ios-backspace-command-test",
			nodes: [FileReferenceNode],
			onError: (error) => {
				throw error;
			},
		});
		let deleteCharacterDirection: boolean | undefined;

		registerIOSBackspaceCommand(editor, true);
		editor.registerCommand(
			DELETE_CHARACTER_COMMAND,
			(isBackward) => {
				deleteCharacterDirection = isBackward;
				return true;
			},
			COMMAND_PRIORITY_HIGH,
		);

		editor.update(
			() => {
				const root = $getRoot();
				const paragraph = $createParagraphNode();
				const fileReference = $createFileReferenceNode(
					"main.go",
					1,
					1,
					"package main",
				);
				const text = $createTextNode("hello");
				root.append(paragraph);
				paragraph.append(fileReference, text);
				text.select(0, 0);
			},
			{ discrete: true },
		);

		const event = new KeyboardEvent("keydown", {
			cancelable: true,
			key: "Backspace",
		});

		expect(editor.dispatchCommand(KEY_BACKSPACE_COMMAND, event)).toBe(true);
		expect(event.defaultPrevented).toBe(true);
		expect(deleteCharacterDirection).toBe(true);
	});

	it("does not prevent default for plain text iOS backspace", () => {
		const editor = createEditor({
			namespace: "ios-backspace-command-test",
			nodes: [FileReferenceNode],
			onError: (error) => {
				throw error;
			},
		});
		let deleteCharacterDirection: boolean | undefined;

		registerIOSBackspaceCommand(editor, true);
		editor.registerCommand(
			DELETE_CHARACTER_COMMAND,
			(isBackward) => {
				deleteCharacterDirection = isBackward;
				return true;
			},
			COMMAND_PRIORITY_HIGH,
		);

		editor.update(
			() => {
				const root = $getRoot();
				const paragraph = $createParagraphNode();
				const text = $createTextNode("hello");
				root.append(paragraph);
				paragraph.append(text);
				text.select(5, 5);
			},
			{ discrete: true },
		);

		const event = new KeyboardEvent("keydown", {
			cancelable: true,
			key: "Backspace",
		});

		expect(editor.dispatchCommand(KEY_BACKSPACE_COMMAND, event)).toBe(true);
		expect(event.defaultPrevented).toBe(false);
		expect(deleteCharacterDirection).toBeUndefined();
	});
});
