import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
	acceptsMessage,
	OriginValidatedTransport,
} from "./originValidatedTransport";

const SANDBOX_ORIGIN = "https://mcp-abc.apps.example.com";

const renderFrame = (): Window => {
	render(<iframe title="sandbox" />);
	const frame = screen.getByTitle("sandbox");
	if (!(frame instanceof HTMLIFrameElement) || !frame.contentWindow) {
		throw new Error("iframe did not mount a window");
	}
	return frame.contentWindow;
};

describe("acceptsMessage", () => {
	it("requires both the pinned source window and origin", () => {
		const frameWindow = renderFrame();
		const event = { source: frameWindow, origin: SANDBOX_ORIGIN };
		expect(acceptsMessage(event, frameWindow, SANDBOX_ORIGIN)).toBe(true);
		expect(acceptsMessage(event, window, SANDBOX_ORIGIN)).toBe(false);
		expect(acceptsMessage(event, frameWindow, "https://evil.example.com")).toBe(
			false,
		);
		expect(
			acceptsMessage(
				{ source: window, origin: SANDBOX_ORIGIN },
				frameWindow,
				SANDBOX_ORIGIN,
			),
		).toBe(false);
		expect(acceptsMessage(event, null, SANDBOX_ORIGIN)).toBe(false);
	});
});

describe("OriginValidatedTransport", () => {
	const dispatch = (source: Window, origin: string, data: unknown) => {
		window.dispatchEvent(new MessageEvent("message", { source, origin, data }));
	};

	it("delivers JSON-RPC messages from the pinned window and origin only", async () => {
		const frameWindow = renderFrame();
		const transport = new OriginValidatedTransport(frameWindow, SANDBOX_ORIGIN);
		const onmessage = vi.fn();
		transport.onmessage = onmessage;
		await transport.start();

		const notification = {
			jsonrpc: "2.0",
			method: "ui/notifications/sandbox-proxy-ready",
			params: {},
		};
		dispatch(frameWindow, SANDBOX_ORIGIN, notification);
		dispatch(frameWindow, "https://evil.example.com", notification);
		dispatch(window, SANDBOX_ORIGIN, notification);

		expect(onmessage).toHaveBeenCalledTimes(1);
		expect(onmessage).toHaveBeenCalledWith(notification);
		await transport.close();
	});

	it("drops payloads that are not JSON-RPC messages", async () => {
		const frameWindow = renderFrame();
		const transport = new OriginValidatedTransport(frameWindow, SANDBOX_ORIGIN);
		const onmessage = vi.fn();
		transport.onmessage = onmessage;
		await transport.start();

		dispatch(frameWindow, SANDBOX_ORIGIN, "hello");
		dispatch(frameWindow, SANDBOX_ORIGIN, { method: "ui/initialize" });
		dispatch(frameWindow, SANDBOX_ORIGIN, null);

		expect(onmessage).not.toHaveBeenCalled();
		await transport.close();
	});

	it("posts outbound messages with the explicit target origin", async () => {
		const frameWindow = renderFrame();
		const postMessage = vi.spyOn(frameWindow, "postMessage");
		const transport = new OriginValidatedTransport(frameWindow, SANDBOX_ORIGIN);
		await transport.start();

		const message = {
			jsonrpc: "2.0" as const,
			method: "ui/notifications/tool-input",
			params: { arguments: {} },
		};
		await transport.send(message);

		expect(postMessage).toHaveBeenCalledWith(message, SANDBOX_ORIGIN);
		await transport.close();
	});

	it("stops listening and reports onclose when closed", async () => {
		const frameWindow = renderFrame();
		const transport = new OriginValidatedTransport(frameWindow, SANDBOX_ORIGIN);
		const onmessage = vi.fn();
		const onclose = vi.fn();
		transport.onmessage = onmessage;
		transport.onclose = onclose;
		await transport.start();
		await transport.close();

		dispatch(frameWindow, SANDBOX_ORIGIN, {
			jsonrpc: "2.0",
			method: "ui/notifications/initialized",
		});

		expect(onclose).toHaveBeenCalledTimes(1);
		expect(onmessage).not.toHaveBeenCalled();
	});
});
