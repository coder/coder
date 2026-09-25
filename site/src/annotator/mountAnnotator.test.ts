import { beforeEach, describe, expect, it, vi } from "vitest";
import { annotatorHostId, mountAnnotator } from "./mountAnnotator";
import { type AnnotationSubmission, maxFieldLength } from "./protocol";

// Our host is the one with a shadow root; a page element sharing the id
// would come first in `getElementById`.
function shadow(): ShadowRoot {
	for (const node of document.querySelectorAll(`#${annotatorHostId}`)) {
		if (node.shadowRoot) {
			return node.shadowRoot;
		}
	}
	throw new Error("annotator not mounted");
}

function query<T extends Element>(selector: string): T {
	const node = shadow().querySelector<T>(selector);
	if (!node) {
		throw new Error(`missing ${selector}`);
	}
	return node;
}

function click(target: Element) {
	target.dispatchEvent(
		new MouseEvent("click", { bubbles: true, cancelable: true }),
	);
}

function comment(target: Element, text: string) {
	click(target);
	const textarea = query<HTMLTextAreaElement>(".popup textarea");
	textarea.value = text;
	textarea.dispatchEvent(new Event("input", { bubbles: true }));
	click(query(".popup .button:not(.outline)"));
}

describe("mountAnnotator", () => {
	beforeEach(() => {
		document.body.innerHTML = `<main><button id="save">Save</button></main>`;
		document.title = "App";
		window.history.replaceState(null, "", "/");
	});

	it("leaves a page element that shares the host id alone", () => {
		document.body.insertAdjacentHTML(
			"afterbegin",
			`<div id="${annotatorHostId}" data-owner="app"></div>`,
		);
		const pageElement = document.querySelector("[data-owner=app]");
		const hosts = () => document.querySelectorAll(`#${annotatorHostId}`);

		const first = mountAnnotator({ document, onSubmit: () => {} });
		expect(pageElement?.isConnected).toBe(true);
		expect(hosts()).toHaveLength(2);

		// Mounting again replaces our host, and only ours.
		const second = mountAnnotator({ document, onSubmit: () => {} });
		expect(pageElement?.isConnected).toBe(true);
		expect(hosts()).toHaveLength(2);
		expect(shadow()).toBeDefined();

		second.destroy();
		first.destroy();
		expect(hosts()).toHaveLength(1);
		expect(pageElement?.isConnected).toBe(true);
	});

	it("only ever stops picking from the toolbar", () => {
		const onStateChange = vi.fn<(state: { picking: boolean }) => void>();
		const handle = mountAnnotator({
			document,
			onSubmit: () => {},
			onStateChange,
		});
		const toolbar = query<HTMLElement>(".toolbar");
		const stop = query<HTMLButtonElement>(".icon-button");
		expect(toolbar.style.display).toBe("none");

		click(stop);
		expect(handle.getState().picking).toBe(false);
		expect(onStateChange).not.toHaveBeenCalledWith({ picking: true });

		handle.setPicking(true);
		expect(toolbar.style.display).toBe("flex");
		expect(stop.getAttribute("aria-label")).toBe("Stop annotating");

		click(stop);
		expect(handle.getState().picking).toBe(false);
		expect(onStateChange).toHaveBeenLastCalledWith({ picking: false });
		expect(toolbar.style.display).toBe("none");
		handle.destroy();
	});

	it("bounds the page title and url it reports", () => {
		const onSubmit = vi.fn<(submission: AnnotationSubmission) => void>();
		const handle = mountAnnotator({ document, onSubmit });
		document.title = "t".repeat(maxFieldLength * 3);
		window.history.replaceState(null, "", `/${"p".repeat(maxFieldLength * 3)}`);
		handle.setPicking(true);

		comment(document.getElementById("save") as HTMLElement, "Bigger");
		expect(onSubmit).toHaveBeenCalledTimes(1);
		const [{ page }] = onSubmit.mock.calls[0];
		expect(page.title).toHaveLength(maxFieldLength);
		expect(page.url).toHaveLength(maxFieldLength);
		expect(page.url.startsWith(window.location.origin)).toBe(true);
		handle.destroy();
	});
});
