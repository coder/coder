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

describe("pending highlights", () => {
	it("holds a quiet ring until the dashboard takes over", async () => {
		document.body.innerHTML = `<button ${annotationIdAttribute}="a">Save</button>`;
		const button = document.querySelector("button") as HTMLElement;
		button.getBoundingClientRect = () => new DOMRect(10, 10, 60, 20);
		const container = document.createElement("div");
		document.body.append(container);
		const layer = createHighlightLayer(document, window, container);
		const item = { id: "a", selector: "button", url: window.location.href };

		layer.markPending(item);
		await nextFrame();
		expect(container.querySelector(".shimmer.pending")).not.toBeNull();

		layer.set([item]);
		await nextFrame();
		expect(container.querySelector(".shimmer.pending")).toBeNull();
		expect(container.querySelectorAll(".shimmer")).toHaveLength(1);

		// Once the working state owns the element, a later pending mark for
		// another annotation does not demote it back to a quiet ring.
		layer.markPending({ ...item, id: "b" });
		expect(container.querySelectorAll(".shimmer")).toHaveLength(1);
		layer.destroy();
	});
});

describe("resolved highlights", () => {
	function working() {
		document.body.innerHTML = `<button ${annotationIdAttribute}="a">Save</button><h1 ${annotationIdAttribute}="b">Title</h1>`;
		for (const node of document.body.querySelectorAll("*")) {
			(node as HTMLElement).getBoundingClientRect = () =>
				new DOMRect(10, 10, 60, 20);
		}
		const container = document.createElement("div");
		document.body.append(container);
		const layer = createHighlightLayer(document, window, container);
		layer.set([
			{ id: "a", selector: "button", url: window.location.href },
			{ id: "b", selector: "h1", url: window.location.href },
		]);
		return { layer, container };
	}

	it("acknowledges only the changed elements, then clears", () => {
		vi.useFakeTimers();
		const { layer, container } = working();
		layer.resolve(["a"]);

		const boxes = container.querySelectorAll(".shimmer");
		expect(boxes).toHaveLength(1);
		expect(boxes[0].classList.contains("resolved")).toBe(true);
		expect(boxes[0].querySelector(".status-chip")?.textContent).toBe("Updated");
		// The untouched annotation's stamp goes with its highlight; the
		// acknowledged one stays put until the acknowledgement clears.
		expect(
			document.querySelector("h1")?.hasAttribute(annotationIdAttribute),
		).toBe(false);
		expect(
			document.querySelector("button")?.hasAttribute(annotationIdAttribute),
		).toBe(true);

		vi.advanceTimersByTime(3000);
		expect(container.querySelectorAll(".shimmer")).toHaveLength(0);
		expect(
			document.querySelector("button")?.hasAttribute(annotationIdAttribute),
		).toBe(false);
		layer.destroy();
		vi.useRealTimers();
	});

	it("clears everything when nothing was changed", () => {
		const { layer, container } = working();
		const cancel = vi.spyOn(window, "cancelAnimationFrame");
		layer.resolve([]);
		expect(container.querySelectorAll(".shimmer")).toHaveLength(0);
		expect(
			document.querySelectorAll(`[${annotationIdAttribute}]`),
		).toHaveLength(0);
		expect(cancel).toHaveBeenCalled();
		layer.destroy();
	});

	it("does not stack chips when acknowledged twice", () => {
		const { layer, container } = working();
		layer.resolve(["a", "b"]);
		layer.resolve(["a"]);
		expect(container.querySelectorAll(".status-chip")).toHaveLength(1);
		layer.destroy();
	});
});
