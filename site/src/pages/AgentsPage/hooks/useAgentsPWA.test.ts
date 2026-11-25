import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useAgentsPWA } from "./useAgentsPWA";

const viewportContent = "width=device-width, initial-scale=1";

describe("useAgentsPWA", () => {
	let viewport: HTMLMetaElement;

	beforeEach(() => {
		viewport = document.createElement("meta");
		viewport.name = "viewport";
		viewport.content = viewportContent;
		document.head.appendChild(viewport);
	});

	afterEach(() => {
		viewport.remove();
	});

	// WCAG 1.4.4: users must be able to pinch-zoom the Agents page.
	it("leaves the viewport zoomable", () => {
		renderHook(() => useAgentsPWA());

		expect(viewport.content).toBe(viewportContent);
	});
});
