// Tests for pasteHelpers utility functions (pure logic, no DOM).
import { beforeAll, describe, expect, it } from "vitest";
import {
	createPasteFile,
	getPasteDataTransfer,
	getPastedPlainText,
	isLargePaste,
} from "./pasteHelpers";

beforeAll(() => {
	if (typeof File.prototype.text !== "function") {
		Object.defineProperty(File.prototype, "text", {
			configurable: true,
			value: function () {
				return new Promise<string>((resolve, reject) => {
					const reader = new FileReader();
					reader.onload = () => resolve(String(reader.result));
					reader.onerror = () => reject(reader.error);
					reader.readAsText(this);
				});
			},
		});
	}
});

type DataTransferLike = Pick<DataTransfer, "getData" | "files">;

const createDataTransfer = (
	values: Record<string, string> = {},
): DataTransfer => {
	const input = document.createElement("input");
	input.type = "file";
	return {
		getData: (type: string) => values[type] ?? "",
		files: input.files!,
	} as DataTransfer;
};

const createClipboardPasteEvent = (
	clipboardData: DataTransferLike,
): ClipboardEvent => {
	const event = new Event("paste") as ClipboardEvent;
	Object.defineProperty(event, "clipboardData", {
		value: clipboardData,
	});
	return event;
};

const createBeforeInputEvent = (
	data?: string,
	dataTransfer?: DataTransferLike,
): InputEvent => {
	const event = new Event("beforeinput") as InputEvent;
	Object.defineProperty(event, "data", {
		value: data ?? null,
	});
	if (dataTransfer) {
		Object.defineProperty(event, "dataTransfer", {
			value: dataTransfer,
		});
	}
	return event;
};

describe("getPasteDataTransfer", () => {
	it("prefers clipboardData from clipboard events", () => {
		const clipboardData = createDataTransfer({
			"text/plain": "from clipboard",
		});
		const event = createClipboardPasteEvent(clipboardData);
		expect(getPasteDataTransfer(event)).toBe(clipboardData);
	});

	it("falls back to dataTransfer for beforeinput paste events", () => {
		const dataTransfer = createDataTransfer({
			"text/plain": "from beforeinput",
		});
		const event = createBeforeInputEvent(undefined, dataTransfer);
		expect(getPasteDataTransfer(event)).toBe(dataTransfer);
	});
});

describe("getPastedPlainText", () => {
	it("reads plain text from clipboard data when available", () => {
		const dataTransfer = createDataTransfer({
			"text/plain": "clipboard",
		});
		const event = createClipboardPasteEvent(dataTransfer);
		expect(getPastedPlainText(event, dataTransfer)).toBe("clipboard");
	});

	it("falls back to InputEvent.data for plain-text paste shortcuts", () => {
		const event = createBeforeInputEvent("paste as plain text");
		expect(getPastedPlainText(event, null)).toBe("paste as plain text");
	});

	it("returns empty string when no plain text is available", () => {
		const dataTransfer = createDataTransfer();
		const event = createBeforeInputEvent();
		expect(getPastedPlainText(event, dataTransfer)).toBe("");
	});
});

describe("isLargePaste", () => {
	it("returns false for short single-line text", () => {
		expect(isLargePaste("Hello world")).toBe(false);
	});

	it("returns false for empty text", () => {
		expect(isLargePaste("")).toBe(false);
	});

	it("returns false for 9 lines of short text", () => {
		const text = Array(9).fill("short line").join("\n");
		expect(isLargePaste(text)).toBe(false);
	});

	it("returns true for 10+ lines", () => {
		const text = Array(10).fill("line").join("\n");
		expect(isLargePaste(text)).toBe(true);
	});

	it("returns true for 1000+ characters even on one line", () => {
		const text = "x".repeat(1000);
		expect(isLargePaste(text)).toBe(true);
	});

	it("returns false for 999 characters on one line", () => {
		const text = "x".repeat(999);
		expect(isLargePaste(text)).toBe(false);
	});

	it("returns false when text has 9 lines and 999 characters", () => {
		const text = [...Array(8).fill("x".repeat(110)), "x".repeat(111)].join(
			"\n",
		);
		expect(text.length).toBe(999);
		expect(isLargePaste(text)).toBe(false);
	});

	it("returns true for text meeting both thresholds", () => {
		const text = Array(15).fill("x".repeat(100)).join("\n");
		expect(isLargePaste(text)).toBe(true);
	});
});

describe("createPasteFile", () => {
	it("creates a File with text/plain type", () => {
		const file = createPasteFile("hello");
		expect(file.type).toBe("text/plain");
		expect(file.name).toMatch(
			/^pasted-text-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}\.txt$/,
		);
	});

	it("preserves the text content", async () => {
		const text = "Hello\nWorld";
		const file = createPasteFile(text);
		const content = await file.text();
		expect(content).toBe(text);
	});
});
