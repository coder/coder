export type PasteCommandEvent = ClipboardEvent | InputEvent;

/**
 * Returns clipboard-style data transfer content from the different event
 * shapes Lexical uses for paste commands.
 */
export function getPasteDataTransfer(
	event: PasteCommandEvent,
): DataTransfer | null {
	if ("clipboardData" in event && event.clipboardData) {
		return event.clipboardData;
	}
	if ("dataTransfer" in event && event.dataTransfer) {
		return event.dataTransfer;
	}
	return null;
}

/**
 * Extracts plain text from a paste command, including beforeinput-based
 * plain-text paste shortcuts such as Cmd/Ctrl+Shift+V.
 */
export function getPastedPlainText(
	event: PasteCommandEvent,
	dataTransfer?: DataTransfer | null,
): string {
	const text = dataTransfer?.getData("text/plain");
	if (text) {
		return text;
	}
	return "data" in event && typeof event.data === "string" ? event.data : "";
}

/**
 * Detects whether the pasted text parses as an SVG document with an
 * `<svg>` root element.
 *
 * The server refuses SVG uploads and probes for an SVG root regardless of
 * the declared MIME type. We can't lean on the clipboard MIME here
 * because SVG source copied from a text editor arrives as `text/plain`,
 * so we ask the browser's XML parser to classify the content for us.
 *
 * The check is deliberately lenient: a false positive just leaves an
 * XML-ish paste inline, which is harmless, whereas a false negative
 * reproduces the "Unsupported file type." bug the server surfaces.
 */
export function hasSVGRootElement(text: string): boolean {
	const doc = new DOMParser().parseFromString(text, "image/svg+xml");
	if (doc.getElementsByTagName("parsererror").length > 0) {
		return false;
	}
	return doc.documentElement?.tagName.toLowerCase() === "svg";
}

/**
 * Determines whether a pasted text should be treated as a file
 * attachment rather than inline editor content.
 *
 * The heuristic: text with 10+ lines OR 1000+ characters is
 * considered "large" and should become an attachment.
 */
export function isLargePaste(text: string): boolean {
	const LINE_THRESHOLD = 10;
	const CHAR_THRESHOLD = 1000;
	// A trailing newline intentionally counts as another line here.
	// Users can use Cmd/Ctrl+Shift+V when they need to force an
	// inline paste instead of creating an attachment.
	const lineCount = text.split("\n").length;
	return lineCount >= LINE_THRESHOLD || text.length >= CHAR_THRESHOLD;
}

/**
 * Creates a synthetic File object from pasted text for the
 * attachment upload pipeline.
 */
export function createPasteFile(text: string): File {
	const now = new Date();
	const pad = (n: number) => String(n).padStart(2, "0");
	const timestamp = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(
		now.getDate(),
	)}-${pad(now.getHours())}-${pad(now.getMinutes())}-${pad(now.getSeconds())}`;
	const fileName = `pasted-text-${timestamp}.txt`;
	return new File([text], fileName, { type: "text/plain" });
}
