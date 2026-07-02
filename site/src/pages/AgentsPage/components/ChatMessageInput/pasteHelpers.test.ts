// Tests for pasteHelpers utility functions (pure logic, no DOM).
import { beforeAll, describe, expect, it } from "vitest";
import {
	createPasteFile,
	getPasteDataTransfer,
	getPastedPlainText,
	hasSVGRootElement,
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

describe("hasSVGRootElement", () => {
	it("returns false for empty text", () => {
		expect(hasSVGRootElement("")).toBe(false);
	});

	it("returns false for pure whitespace", () => {
		expect(hasSVGRootElement("   \n\t ")).toBe(false);
	});

	it("detects a bare <svg> element", () => {
		expect(hasSVGRootElement("<svg></svg>")).toBe(true);
	});

	it("detects a self-closing <svg/> element", () => {
		expect(hasSVGRootElement("<svg/>")).toBe(true);
	});

	it("detects <svg> with attributes", () => {
		expect(
			hasSVGRootElement(
				'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"/>',
			),
		).toBe(true);
	});

	it("is case-insensitive on the tag name", () => {
		expect(hasSVGRootElement("<SVG></SVG>")).toBe(true);
		expect(hasSVGRootElement("<SvG></SvG>")).toBe(true);
	});

	it("detects <svg> after an XML declaration", () => {
		expect(
			hasSVGRootElement(
				'<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>',
			),
		).toBe(true);
	});

	it("detects <svg> after an XML declaration with whitespace", () => {
		expect(
			hasSVGRootElement('<?xml version="1.0" encoding="UTF-8"?>\n<svg></svg>'),
		).toBe(true);
	});

	it("detects <svg> after a DOCTYPE directive", () => {
		expect(
			hasSVGRootElement(
				'<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">\n<svg></svg>',
			),
		).toBe(true);
	});

	it("detects <svg> after XML comments", () => {
		expect(hasSVGRootElement("<!-- created by hand -->\n<svg></svg>")).toBe(
			true,
		);
	});

	it("skips a UTF-8 BOM before <svg>", () => {
		expect(hasSVGRootElement("\uFEFF<svg></svg>")).toBe(true);
	});

	it("handles combined prolog: BOM, XML decl, comment, DOCTYPE, then <svg>", () => {
		const text = [
			"\uFEFF",
			'<?xml version="1.0"?>',
			"<!-- hand written -->",
			"<!DOCTYPE svg>",
			'<svg xmlns="http://www.w3.org/2000/svg"/>',
		].join("\n");
		expect(hasSVGRootElement(text)).toBe(true);
	});

	it("returns false for HTML documents", () => {
		expect(hasSVGRootElement("<html><body>not svg</body></html>")).toBe(false);
	});

	it("returns false for markdown that mentions svg later", () => {
		expect(hasSVGRootElement('# SVG Example\n<svg width="100">...</svg>')).toBe(
			false,
		);
	});

	it("returns false for CSV rows containing svg fragments", () => {
		expect(hasSVGRootElement("name,icon\nlogo,<svg><rect/></svg>\n")).toBe(
			false,
		);
	});

	it("returns false for lookalike tag names such as <svgx>", () => {
		expect(hasSVGRootElement("<svgx></svgx>")).toBe(false);
	});

	it("returns false for plain text starting with '<'", () => {
		expect(hasSVGRootElement("< less than sign followed by text")).toBe(false);
	});

	it("returns false for unterminated XML processing instructions", () => {
		expect(hasSVGRootElement("<?xml version='1.0'")).toBe(false);
	});

	it("returns false for unterminated XML comments", () => {
		expect(hasSVGRootElement("<!-- open comment")).toBe(false);
	});
});
