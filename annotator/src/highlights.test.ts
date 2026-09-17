import { describe, expect, it, vi } from "vitest";
import { createHighlightLayer } from "./highlights";
import { annotationIdAttribute } from "./protocol";

const nextFrame = () =>
	new Promise<void>((resolve) => {
		requestAnimationFrame(() => resolve());
	});

function setup(html: string) {
	document.body.innerHTML = html;
	const container = document.createElement("div");
	document.body.append(container);
	const layer = createHighlightLayer(document, window, container);
	return { layer, container };
}

describe("highlight layer", () => {
	it("follows the stamped element rather than the selector", async () => {
		const { layer, container } = setup(
			`<h1 id="other">Other</h1><h1 ${annotationIdAttribute}="a">Mine</h1>`,
		);
		const mine = document.querySelector(`[${annotationIdAttribute}]`);
		if (!(mine instanceof HTMLElement)) {
			throw new Error("missing stamped element");
		}
		mine.getBoundingClientRect = () => new DOMRect(100, 200, 50, 20);
		layer.set([{ id: "a", selector: "h1", url: window.location.href }]);
		await nextFrame();
		const box = container.firstElementChild as HTMLElement;
		expect(box.style.display).toBe("block");
		expect(box.style.left).toBe("96px");
		layer.destroy();
		expect(mine.hasAttribute(annotationIdAttribute)).toBe(false);
	});

	it("survives ids that are not valid selector text", async () => {
		const { layer, container } = setup("<h1>Title</h1>");
		const h1 = document.querySelector("h1") as HTMLElement;
		h1.setAttribute(annotationIdAttribute, 'we"ird]');
		h1.getBoundingClientRect = () => new DOMRect(10, 10, 50, 20);
		layer.set([{ id: 'we"ird]', selector: "h1", url: window.location.href }]);
		await nextFrame();
		const box = container.firstElementChild as HTMLElement;
		expect(box.style.display).toBe("block");
		layer.destroy();
		expect(h1.hasAttribute(annotationIdAttribute)).toBe(false);
	});

	it("leaves stamps it does not own alone", async () => {
		const { layer } = setup(
			`<h1 ${annotationIdAttribute}="theirs">Title</h1><p ${annotationIdAttribute}="a">x</p>`,
		);
		layer.set([{ id: "a", selector: "p", url: window.location.href }]);
		await nextFrame();
		layer.destroy();
		expect(
			document.querySelector("h1")?.getAttribute(annotationIdAttribute),
		).toBe("theirs");
		expect(
			document.querySelector("p")?.hasAttribute(annotationIdAttribute),
		).toBe(false);
	});

	it("stops the frame loop when destroyed", async () => {
		const { layer } = setup("<h1>Title</h1>");
		layer.set([{ id: "a", selector: "h1", url: window.location.href }]);
		await nextFrame();
		const cancel = vi.spyOn(window, "cancelAnimationFrame");
		layer.destroy();
		expect(cancel).toHaveBeenCalled();
	});

	it("does not fall back to the selector on another page", async () => {
		const { layer, container } = setup("<h1>Same shape</h1>");
		const h1 = document.querySelector("h1") as HTMLElement;
		h1.getBoundingClientRect = () => new DOMRect(0, 0, 50, 20);
		layer.set([
			{ id: "a", selector: "h1", url: "http://localhost/some/other/page" },
		]);
		await nextFrame();
		const box = container.firstElementChild as HTMLElement;
		expect(box.style.display).toBe("none");
		layer.destroy();
	});
});
