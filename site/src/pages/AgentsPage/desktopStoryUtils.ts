import { fn } from "storybook/test";
import type { UseDesktopConnectionResult } from "./useDesktopConnection";

/**
 * Creates a mock attach function that inserts a placeholder element
 * simulating a noVNC canvas so the "connected" state is visible in
 * stories.
 */
export function mockAttach(): (container: HTMLElement) => void {
	const placeholder = document.createElement("div");
	Object.assign(placeholder.style, {
		width: "100%",
		height: "100%",
		background:
			"linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)",
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
		color: "#888",
		fontSize: "14px",
		fontFamily: "monospace",
	});
	placeholder.textContent = "VNC canvas placeholder";

	const attachFn = fn((container: HTMLElement) => {
		if (placeholder.parentElement !== container) {
			container.appendChild(placeholder);
		}
	});

	return attachFn;
}

export function mockDesktopConnection(
	overrides: Partial<UseDesktopConnectionResult> = {},
): UseDesktopConnectionResult {
	return {
		status: "idle",
		hasConnected: false,
		connect: fn(),
		disconnect: fn(),
		attach: fn(),
		rfb: null,
		...overrides,
	};
}
