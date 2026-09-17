import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { annotatorHostId, mountAnnotator } from "./mountAnnotator";
import { type AnnotationSubmission, maxFieldLength } from "./protocol";

// The overlay renders into a shadow root, which Testing Library's queries
// do not reach, so its controls are located by their accessible
// attributes directly. Our host is the one with a shadow root; a page
// element sharing the id would come first in `getElementById`.
function shadow(): ShadowRoot {
	for (const node of document.querySelectorAll(`#${annotatorHostId}`)) {
		if (node.shadowRoot) {
			return node.shadowRoot;
		}
	}
	throw new Error("annotator not mounted");
}

function control<T extends HTMLElement>(selector: string): T {
	const node = shadow().querySelector<T>(selector);
	if (!node) {
		throw new Error(`missing ${selector}`);
	}
	return node;
}

const dialog = () => shadow().querySelector('[role="dialog"]');
const saveButton = () => screen.getByRole("button", { name: "Save" });

async function comment(text: string) {
	await userEvent.click(saveButton());
	await userEvent.type(control('[aria-label="Annotation comment"]'), text);
	await userEvent.click(control('[role="dialog"] .button:not(.outline)'));
}

describe("mountAnnotator", () => {
	beforeEach(() => {
		document.body.innerHTML = `<main><button id="save">Save</button></main>`;
		document.title = "App";
		window.history.replaceState(null, "", "/");
	});

	it("tears down an earlier mount and leaves a page element sharing the host id alone", () => {
		document.body.insertAdjacentHTML(
			"afterbegin",
			`<div id="${annotatorHostId}" data-owner="app"></div>`,
		);
		const pageElement = document.querySelector("[data-owner=app]");
		const hosts = () => document.querySelectorAll(`#${annotatorHostId}`);
		const onStateChange = vi.fn<(state: { picking: boolean }) => void>();

		const first = mountAnnotator({
			document,
			onSubmit: () => {},
			onStateChange,
		});
		first.setPicking(true);
		expect(hosts()).toHaveLength(2);

		const second = mountAnnotator({ document, onSubmit: () => {} });
		expect(first.getState().picking).toBe(false);
		expect(onStateChange).toHaveBeenLastCalledWith({ picking: false });
		expect(hosts()).toHaveLength(2);
		expect(pageElement?.isConnected).toBe(true);

		second.destroy();
		first.destroy();
		expect(hosts()).toHaveLength(1);
		expect(pageElement?.isConnected).toBe(true);
	});

	it("only ever stops picking from the toolbar", async () => {
		const onStateChange = vi.fn<(state: { picking: boolean }) => void>();
		const handle = mountAnnotator({
			document,
			onSubmit: () => {},
			onStateChange,
		});
		const stop = control('[aria-label="Stop annotating"]');

		await userEvent.click(stop);
		expect(handle.getState().picking).toBe(false);
		expect(onStateChange).not.toHaveBeenCalledWith({ picking: true });

		handle.setPicking(true);
		await userEvent.click(stop);
		expect(handle.getState().picking).toBe(false);
		expect(onStateChange).toHaveBeenLastCalledWith({ picking: false });
		handle.destroy();
	});

	it("keeps the page's own handlers from seeing picks", async () => {
		const pageClick = vi.fn();
		saveButton().addEventListener("click", pageClick);
		const handle = mountAnnotator({ document, onSubmit: () => {} });

		handle.setPicking(true);
		await userEvent.click(saveButton());
		expect(pageClick).not.toHaveBeenCalled();
		expect(dialog()).not.toBeNull();

		handle.setPicking(false);
		expect(dialog()).toBeNull();
		await userEvent.click(saveButton());
		expect(pageClick).toHaveBeenCalledTimes(1);
		handle.destroy();
	});

	it("closes an open comment on Escape before leaving annotate mode", async () => {
		const onSubmit = vi.fn();
		const handle = mountAnnotator({ document, onSubmit });
		handle.setPicking(true);
		await userEvent.click(saveButton());

		await userEvent.keyboard("{Escape}");
		expect(dialog()).toBeNull();
		expect(handle.getState().picking).toBe(true);

		await userEvent.keyboard("{Escape}");
		expect(handle.getState().picking).toBe(false);
		expect(onSubmit).not.toHaveBeenCalled();
		handle.destroy();
	});

	it("sends each comment with the picked element and bounded page details", async () => {
		const onSubmit = vi.fn<(submission: AnnotationSubmission) => void>();
		const handle = mountAnnotator({ document, onSubmit });
		document.title = "t".repeat(maxFieldLength * 3);
		window.history.replaceState(null, "", `/${"p".repeat(maxFieldLength * 3)}`);
		handle.setPicking(true);

		await comment("Bigger");
		expect(onSubmit).toHaveBeenCalledTimes(1);
		const [{ page, annotations }] = onSubmit.mock.calls[0];
		expect(annotations).toHaveLength(1);
		expect(annotations[0]).toMatchObject({
			comment: "Bigger",
			element: { tag: "button", selector: "#save", text: "Save" },
		});
		expect(page.title).toHaveLength(maxFieldLength);
		expect(page.url).toHaveLength(maxFieldLength);
		expect(page.url.startsWith(window.location.origin)).toBe(true);
		expect(dialog()).toBeNull();
		handle.destroy();
	});
});
