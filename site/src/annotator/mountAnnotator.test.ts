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
const heldBadge = () => control('[aria-label="Discard held comments"]');

// One user session per comment so a held Shift carries into the click.
async function comment(target: Element, text: string, hold = false) {
	const user = userEvent.setup();
	await user.click(target);
	await user.type(control('[aria-label="Annotation comment"]'), text);
	if (hold) {
		await user.keyboard("{Shift>}");
	}
	await user.click(control('[role="dialog"] .button:not(.outline)'));
	if (hold) {
		await user.keyboard("{/Shift}");
	}
}

describe("mountAnnotator", () => {
	beforeEach(() => {
		document.body.innerHTML = `<main><h1 id="title">Hi</h1><button id="save">Save</button></main>`;
		for (const node of document.querySelectorAll("main *")) {
			(node as HTMLElement).getBoundingClientRect = () =>
				new DOMRect(10, 10, 100, 30);
		}
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

		await comment(saveButton(), "Bigger");
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

	it("sends held comments together with the next Send", async () => {
		const onSubmit = vi.fn<(submission: AnnotationSubmission) => void>();
		const handle = mountAnnotator({ document, onSubmit });
		handle.setPicking(true);

		await comment(screen.getByRole("heading", { name: "Hi" }), "Bigger", true);
		expect(onSubmit).not.toHaveBeenCalled();
		expect(heldBadge().textContent).toBe("1");
		expect(shadow().querySelectorAll(".held-outline")).toHaveLength(1);
		expect(handle.getState().picking).toBe(true);

		await comment(saveButton(), "Primary");
		expect(onSubmit).toHaveBeenCalledTimes(1);
		const [submission] = onSubmit.mock.calls[0];
		expect(submission.annotations.map((a) => a.comment)).toEqual([
			"Bigger",
			"Primary",
		]);
		expect(submission.annotations.map((a) => a.element.selector)).toEqual([
			"#title",
			"#save",
		]);
		expect(shadow().querySelectorAll(".held-outline")).toHaveLength(0);
		expect(heldBadge().style.display).toBe("none");
		handle.destroy();
	});

	it("discards held comments from the toolbar badge", async () => {
		const onSubmit = vi.fn();
		const handle = mountAnnotator({ document, onSubmit });
		handle.setPicking(true);
		const title = screen.getByRole("heading", { name: "Hi" });
		await comment(title, "Bigger", true);
		await userEvent.click(heldBadge());
		expect(shadow().querySelectorAll(".held-outline")).toHaveLength(0);
		expect(title.hasAttribute("data-coder-annotation-id")).toBe(false);
		expect(onSubmit).not.toHaveBeenCalled();
		handle.destroy();
	});
});
