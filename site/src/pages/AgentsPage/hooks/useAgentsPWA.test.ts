import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useAgentsPWA } from "./useAgentsPWA";

describe("useAgentsPWA", () => {
	afterEach(() => {
		document.head.innerHTML = "";
	});

	it("points the manifest at the Agents app while mounted", () => {
		const manifest = document.createElement("link");
		manifest.rel = "manifest";
		manifest.href = "/manifest.webmanifest";
		document.head.appendChild(manifest);

		const { unmount } = renderHook(() => useAgentsPWA());

		expect(document.querySelectorAll('link[rel="manifest"]')).toHaveLength(1);
		expect(manifest.getAttribute("href")).toBe("/agents/manifest.webmanifest");

		unmount();

		expect(manifest.getAttribute("href")).toBe("/manifest.webmanifest");
	});
});
