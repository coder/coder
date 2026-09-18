import { beforeEach, describe, expect, it, vi } from "vitest";
import { mountAnnotator } from "./mountAnnotator";
import type { AnnotationSubmission } from "./protocol";

function shadow() {
	const root = document.getElementById("coder-annotator-host")?.shadowRoot;
	if (!root) {
		throw new Error("annotator not mounted");
	}
	return root;
}

function query<T extends Element>(selector: string): T {
	const node = shadow().querySelector<T>(selector);
	if (!node) {
		throw new Error(`missing ${selector}`);
	}
	return node;
}

function click(target: Element, init: MouseEventInit = {}) {
	target.dispatchEvent(
		new MouseEvent("click", { bubbles: true, cancelable: true, ...init }),
	);
}

function comment(target: Element, text: string, hold: boolean) {
	click(target);
	const textarea = query<HTMLTextAreaElement>(".popup textarea");
	textarea.value = text;
	textarea.dispatchEvent(new Event("input", { bubbles: true }));
	click(query(".popup .button:not(.outline)"), { shiftKey: hold });
}

describe("mountAnnotator", () => {
	beforeEach(() => {
		document.body.innerHTML = `<main><h1 id="title">Hi</h1><button id="save">Save</button></main>`;
		for (const node of document.querySelectorAll("main *")) {
			(node as HTMLElement).getBoundingClientRect = () =>
				new DOMRect(10, 10, 100, 30);
		}
	});

	it("sends held comments together with the next Send", () => {
		const onSubmit = vi.fn<(submission: AnnotationSubmission) => void>();
		const handle = mountAnnotator({ document, onSubmit });
		handle.setPicking(true);
		const title = document.getElementById("title") as HTMLElement;
		const save = document.getElementById("save") as HTMLElement;

		comment(title, "Bigger", true);
		expect(onSubmit).not.toHaveBeenCalled();
		expect(query(".held-badge").textContent).toBe("1");
		expect(shadow().querySelectorAll(".held-outline")).toHaveLength(1);
		expect(handle.getState().picking).toBe(true);

		comment(save, "Primary", false);
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
		expect(query<HTMLElement>(".held-badge").style.display).toBe("none");
		handle.destroy();
	});

	it("discards held comments from the toolbar badge", () => {
		const onSubmit = vi.fn();
		const handle = mountAnnotator({ document, onSubmit });
		handle.setPicking(true);
		comment(document.getElementById("title") as HTMLElement, "Bigger", true);
		click(query(".held-badge"));
		expect(shadow().querySelectorAll(".held-outline")).toHaveLength(0);
		expect(
			document
				.getElementById("title")
				?.hasAttribute("data-coder-annotation-id"),
		).toBe(false);
		handle.destroy();
	});
});
