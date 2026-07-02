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
 * Detects whether the pasted text begins with an SVG root element,
 * mirroring the server-side chatfiles.HasSVGRootElement classifier.
 *
 * The server explicitly refuses SVG uploads for security reasons (SVG can
 * carry active script content), so we intercept SVG XML in the paste
 * pipeline before the large-paste-to-file conversion would upload it as an
 * attachment.
 *
 * Only structural markup, that is, XML declarations, processing
 * instructions, comments, DOCTYPE-like directives, and whitespace char
 * data, is allowed before the root element. Any other leading content
 * makes this return false, matching the Go xml decoder semantics used by
 * ClassifyStoredMediaType in coderd/x/chatfiles/mime.go.
 */
export function hasSVGRootElement(text: string): boolean {
	// Strip a UTF-8 BOM. Some editors add one when saving.
	let remaining = text.startsWith("\uFEFF") ? text.slice(1) : text;

	while (remaining.length > 0) {
		const trimmed = remaining.replace(/^\s+/, "");
		if (trimmed !== remaining) {
			remaining = trimmed;
			continue;
		}

		// XML processing instruction: <?target ... ?>
		if (remaining.startsWith("<?")) {
			const end = remaining.indexOf("?>");
			if (end === -1) return false;
			remaining = remaining.slice(end + 2);
			continue;
		}

		// XML comment: <!-- ... -->
		if (remaining.startsWith("<!--")) {
			const end = remaining.indexOf("-->", 4);
			if (end === -1) return false;
			remaining = remaining.slice(end + 3);
			continue;
		}

		// XML directive (e.g. <!DOCTYPE ...>). Bare heuristic; anything
		// past the first '>' is treated as consumed. This matches how
		// the Go decoder skips over xml.Directive tokens between the
		// prolog and the root element.
		if (remaining.startsWith("<!")) {
			const end = remaining.indexOf(">");
			if (end === -1) return false;
			remaining = remaining.slice(end + 1);
			continue;
		}

		// First non-structural token must be the SVG root opening tag.
		// The trailing character class rejects lookalike names such as
		// <svgx> while accepting <svg>, <svg/> and <svg ...>.
		return /^<svg[\s/>]/i.test(remaining);
	}

	return false;
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
