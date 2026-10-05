import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useAgentsPWA } from "./useAgentsPWA";

describe("useAgentsPWA", () => {
	it("does not ask iOS to draw the page under the status bar", () => {
		renderHook(() => useAgentsPWA());

		expect(
			document.head.querySelector('meta[name="apple-mobile-web-app-capable"]'),
		).not.toBeNull();
		expect(
			document.head.querySelector(
				'meta[name="apple-mobile-web-app-status-bar-style"]',
			),
		).toBeNull();
	});
});
