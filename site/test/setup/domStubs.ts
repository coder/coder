// Monaco editor calls document.queryCommandSupported at import time,
// which is absent from JSDOM.
if (typeof document.queryCommandSupported !== "function") {
	document.queryCommandSupported = () => false;
}

// Pointer capture stubs required for Radix UI in JSDOM.
globalThis.HTMLElement.prototype.hasPointerCapture = vi
	.fn()
	.mockReturnValue(false);
globalThis.HTMLElement.prototype.releasePointerCapture = vi.fn();
globalThis.HTMLElement.prototype.scrollIntoView = vi.fn();
globalThis.HTMLElement.prototype.setPointerCapture = vi.fn();

// Lexical measures the selection when text is entered, and JSDOM does
// not implement Range geometry.
globalThis.Range.prototype.getBoundingClientRect = () =>
	new DOMRect(0, 0, 1, 16);
