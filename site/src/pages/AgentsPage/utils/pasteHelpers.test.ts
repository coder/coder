import { describe, expect, it } from "vitest";
import {
	createPasteFile,
	getPasteDataTransfer,
	getPastedPlainText,
	isLargePaste,
	type PasteCommandEvent,
} from "./pasteHelpers";

describe("getPasteDataTransfer", () => {
	it("prefers clipboardData from clipboard events", () => {
		const clipboardData = {
			getData: () => "from clipboard",
			files: [],
		} as unknown as DataTransfer;
		const event = {
			clipboardData,
		} as PasteCommandEvent;
		expect(getPasteDataTransfer(event)).toBe(clipboardData);
	});

	it("falls back to dataTransfer for beforeinput paste events", () => {
		const dataTransfer = {
			getData: () => "from beforeinput",
			files: [],
		} as unknown as DataTransfer;
		const event = {
			dataTransfer,
		} as PasteCommandEvent;
		expect(getPasteDataTransfer(event)).toBe(dataTransfer);
	});
});

describe("getPastedPlainText", () => {
	it("reads plain text from clipboard data when available", () => {
		const dataTransfer = {
			getData: (type: string) => (type === "text/plain" ? "clipboard" : ""),
		} as unknown as DataTransfer;
		const event = {
			clipboardData: dataTransfer,
		} as PasteCommandEvent;
		expect(getPastedPlainText(event, dataTransfer)).toBe("clipboard");
	});

	it("falls back to InputEvent.data for plain-text paste shortcuts", () => {
		const event = {
			data: "paste as plain text",
		} as PasteCommandEvent;
		expect(getPastedPlainText(event, null)).toBe("paste as plain text");
	});

	it("returns empty string when no plain text is available", () => {
		const dataTransfer = {
			getData: () => "",
		} as unknown as DataTransfer;
		const event = {} as PasteCommandEvent;
		expect(getPastedPlainText(event, dataTransfer)).toBe("");
	});
});

describe("isLargePaste", () => {
	it("returns false for short single-line text", () => {
		expect(isLargePaste("Hello world")).toBe(false);
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
