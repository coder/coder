// Pointer capture stubs required for Radix UI in JSDOM.
globalThis.HTMLElement.prototype.hasPointerCapture = vi
	.fn()
	.mockReturnValue(false);
globalThis.HTMLElement.prototype.releasePointerCapture = vi.fn();
globalThis.HTMLElement.prototype.scrollIntoView = vi.fn();
globalThis.HTMLElement.prototype.setPointerCapture = vi.fn();
