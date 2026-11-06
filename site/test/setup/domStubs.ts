// Monaco editor calls document.queryCommandSupported at import time,
// which is absent from JSDOM.
if (typeof document.queryCommandSupported !== "function") {
	document.queryCommandSupported = () => false;
}

// Pointer capture stubs required for Radix UI in JSDOM.
globalThis.HTMLElement.prototype.hasPointerCapture = vi.fn(() => false);
globalThis.HTMLElement.prototype.releasePointerCapture = vi.fn();
globalThis.HTMLElement.prototype.setPointerCapture = vi.fn();

// JSDOM does not implement scrolling.
globalThis.HTMLElement.prototype.scrollIntoView = vi.fn();
globalThis.Element.prototype.scrollTo = vi.fn();

// vi.restoreAllMocks() does not undo a test's override of these shared stubs.
afterEach(() => {
	vi.mocked(HTMLElement.prototype.hasPointerCapture).mockReset();
	vi.mocked(HTMLElement.prototype.releasePointerCapture).mockReset();
	vi.mocked(HTMLElement.prototype.setPointerCapture).mockReset();
	vi.mocked(HTMLElement.prototype.scrollIntoView).mockReset();
	vi.mocked(Element.prototype.scrollTo).mockReset();
});
