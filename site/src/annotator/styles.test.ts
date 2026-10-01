import { describe, expect, it } from "vitest";
import { annotatorHostId, mountAnnotator } from "./mountAnnotator";

describe("annotator styles", () => {
	it("mounts the real stylesheet into the shadow root", () => {
		const handle = mountAnnotator({ document, onSubmit: () => {} });
		const style = document
			.getElementById(annotatorHostId)
			?.shadowRoot?.querySelector("style");
		// The stylesheet is a CSS file imported inline. Vitest stubs CSS
		// imports unless told otherwise, and nothing else would notice the
		// overlay mounting unstyled.
		expect(style?.textContent).toContain(":host");
		expect(style?.textContent).toContain(".highlight");
		handle.destroy();
	});
});
