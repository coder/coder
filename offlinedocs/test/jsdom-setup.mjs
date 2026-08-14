// jsdom environment for the offlinedocs component tests, loaded as a Node
// --import preload so the DOM globals exist before React DOM and fumadocs-ui
// are imported by any test file. Kept as plain .mjs outside src/ so it is not
// part of the TypeScript build and needs no jsdom type stubs.
//
// The component under test (OSTab) drives fumadocs-ui's Radix-based Tabs, which
// expect a handful of browser globals jsdom does not provide on its own
// (ResizeObserver, matchMedia, requestAnimationFrame, scrollIntoView). They are
// stubbed minimally: the tests assert tab-panel state, not layout or animation.
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><body></body></html>", {
	url: "https://offlinedocs.test/",
	pretendToBeVisual: true,
});
const { window } = dom;

for (const [key, value] of [
	["window", window],
	["document", window.document],
	["navigator", window.navigator],
	["sessionStorage", window.sessionStorage],
	["localStorage", window.localStorage],
]) {
	Object.defineProperty(globalThis, key, { value, configurable: true });
}

globalThis.HTMLElement = window.HTMLElement;
globalThis.Node = window.Node;
globalThis.getComputedStyle = window.getComputedStyle.bind(window);

class ResizeObserverStub {
	observe() {}
	unobserve() {}
	disconnect() {}
}
window.ResizeObserver = ResizeObserverStub;
globalThis.ResizeObserver = ResizeObserverStub;

window.matchMedia ||= () => ({
	matches: false,
	media: "",
	onchange: null,
	addEventListener() {},
	removeEventListener() {},
	addListener() {},
	removeListener() {},
	dispatchEvent() {
		return false;
	},
});
globalThis.matchMedia = window.matchMedia;

globalThis.requestAnimationFrame = (cb) => setTimeout(() => cb(Date.now()), 0);
globalThis.cancelAnimationFrame = (id) => clearTimeout(id);
window.requestAnimationFrame = globalThis.requestAnimationFrame;
window.cancelAnimationFrame = globalThis.cancelAnimationFrame;

window.Element.prototype.scrollIntoView ||= () => {};

// React's test helpers expect this flag when act() is used to flush effects.
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
