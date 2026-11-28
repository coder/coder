import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type {
	PreviewSignInRequest,
	PreviewSignInResponse,
} from "#/api/typesGenerated";
import { getPreferredProxy, ProxyContext } from "#/contexts/ProxyContext";
import {
	MockPreviewAddressResponse,
	MockPreviewSignInResponse,
} from "#/testHelpers/chatEntities";
import { MockHealthyWildWorkspaceProxy } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { PreviewFrame } from "./PreviewFrame";

const mockPreviewRequest: Omit<PreviewSignInRequest, "region_id"> = {
	workspace_id: "workspace-1",
	agent_name: "main",
	preview_slug: "preview--3000",
	url_path: "/docs",
};

const { origin } = MockPreviewAddressResponse;
const signInNeeded = "coder-preview-sign-in-needed";

const renderPreviewFrame = (title: string) =>
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<ProxyContext
				value={{
					proxy: getPreferredProxy(
						[MockHealthyWildWorkspaceProxy],
						MockHealthyWildWorkspaceProxy,
					),
					proxies: [MockHealthyWildWorkspaceProxy],
					proxyLatencies: {},
					latenciesLoaded: true,
					isLoading: false,
					isFetched: true,
					setProxy: vi.fn(),
					clearProxy: vi.fn(),
					refetchProxyLatencies: () => new Date(),
				}}
			>
				<PreviewFrame preview={mockPreviewRequest} title={title} />
			</ProxyContext>
		</QueryClientProvider>,
	);

// The wrapper is a generated document, not part of the rendered page, so it
// can only be inspected by parsing it.
const parseWrapper = (frame: HTMLElement) =>
	new DOMParser().parseFromString(
		frame.getAttribute("srcdoc") ?? "",
		"text/html",
	);

const innerFrameSrc = (frame: HTMLElement) =>
	parseWrapper(frame).querySelector("iframe")?.getAttribute("src");

// jsdom does not load srcdoc, so the test adds the wrapper's preview frame
// itself. Its window is the one the preview host's page posts from.
const previewWindow = (frame: HTMLElement): Window => {
	const wrapper = (frame as HTMLIFrameElement).contentDocument;
	if (!wrapper) {
		throw new Error("the frame has no document");
	}
	const inner = wrapper.body.appendChild(wrapper.createElement("iframe"));
	if (!inner.contentWindow) {
		throw new Error("the preview frame has no window");
	}
	return inner.contentWindow;
};

const post = (data: unknown, messageOrigin: string, source: Window) =>
	window.dispatchEvent(
		new MessageEvent("message", { data, origin: messageOrigin, source }),
	);

// A sign-in request starts after a few promise turns, so a check that none
// started waits for them first.
const settle = () =>
	act(() => new Promise<void>((resolve) => setTimeout(resolve, 0)));

afterEach(() => {
	vi.restoreAllMocks();
});

describe("PreviewFrame", () => {
	it("loads the preview's address through the selected proxy without signing in, framing only its origin in the server's sandbox", async () => {
		const address: PreviewSignInResponse = {
			...MockPreviewAddressResponse,
			url: `${origin}/docs?a="1"&b=<x>`,
		};
		const title = '"><script>document.title="x"</script>';
		const getAddress = vi
			.spyOn(API.experimental, "getPreviewAddress")
			.mockResolvedValue(address);
		const signIn = vi.spyOn(API.experimental, "signInToPreview");
		renderPreviewFrame(title);

		const frame = await screen.findByTitle(title);
		const wrapper = parseWrapper(frame);
		const innerFrames = wrapper.querySelectorAll("iframe");

		expect(getAddress).toHaveBeenCalledWith({
			...mockPreviewRequest,
			region_id: MockHealthyWildWorkspaceProxy.id,
		});
		expect(signIn).not.toHaveBeenCalled();
		expect(frame.getAttribute("sandbox")).toBe(address.sandbox);
		expect(
			wrapper
				.querySelector('meta[http-equiv="Content-Security-Policy"]')
				?.getAttribute("content"),
		).toBe(
			`default-src 'none'; style-src 'unsafe-inline'; frame-src ${origin}`,
		);
		expect(wrapper.querySelectorAll("script")).toHaveLength(0);
		expect(innerFrames).toHaveLength(1);
		expect(innerFrames[0].getAttribute("src")).toBe(address.url);
		expect(innerFrames[0].getAttribute("title")).toBe(title);
		expect(innerFrames[0].getAttribute("referrerpolicy")).toBe("no-referrer");
	});

	it("signs in once when the preview's own page says it is signed out", async () => {
		vi.spyOn(API.experimental, "getPreviewAddress").mockResolvedValue(
			MockPreviewAddressResponse,
		);
		const signIn = vi
			.spyOn(API.experimental, "signInToPreview")
			.mockResolvedValue(MockPreviewSignInResponse);
		renderPreviewFrame("Docs site");
		const frame = await screen.findByTitle("Docs site");
		const preview = previewWindow(frame);

		// Only the preview's window on the preview's origin may ask.
		post(signInNeeded, "https://other.example.com", preview);
		post(signInNeeded, origin, window);
		post("something else", origin, preview);
		await settle();
		expect(signIn).not.toHaveBeenCalled();

		post(signInNeeded, origin, preview);
		post(signInNeeded, origin, preview);
		await waitFor(() =>
			expect(innerFrameSrc(screen.getByTitle("Docs site"))).toBe(
				MockPreviewSignInResponse.url,
			),
		);
		expect(signIn).toHaveBeenCalledTimes(1);
		expect(signIn).toHaveBeenCalledWith({
			...mockPreviewRequest,
			region_id: MockHealthyWildWorkspaceProxy.id,
		});

		// Once per mount: the signed-in frame asking again changes nothing.
		post(signInNeeded, origin, previewWindow(screen.getByTitle("Docs site")));
		await settle();
		expect(signIn).toHaveBeenCalledTimes(1);
	});

	it("reloads at the address without signing in", async () => {
		vi.spyOn(API.experimental, "getPreviewAddress").mockResolvedValue(
			MockPreviewAddressResponse,
		);
		const signIn = vi
			.spyOn(API.experimental, "signInToPreview")
			.mockResolvedValue(MockPreviewSignInResponse);
		renderPreviewFrame("Docs site");
		post(
			signInNeeded,
			origin,
			previewWindow(await screen.findByTitle("Docs site")),
		);
		await waitFor(() =>
			expect(innerFrameSrc(screen.getByTitle("Docs site"))).toBe(
				MockPreviewSignInResponse.url,
			),
		);

		await userEvent.click(
			screen.getByRole("button", { name: "Reload preview" }),
		);

		expect(innerFrameSrc(screen.getByTitle("Docs site"))).toBe(
			MockPreviewAddressResponse.url,
		);
		await settle();
		expect(signIn).toHaveBeenCalledTimes(1);
	});

	it("reloads after the address request fails", async () => {
		const getAddress = vi
			.spyOn(API.experimental, "getPreviewAddress")
			.mockRejectedValueOnce(new Error("address failed"))
			.mockResolvedValue(MockPreviewAddressResponse);
		renderPreviewFrame("Docs site");
		await screen.findByRole("alert");

		await userEvent.click(
			screen.getByRole("button", { name: "Reload preview" }),
		);

		expect(innerFrameSrc(await screen.findByTitle("Docs site"))).toBe(
			MockPreviewAddressResponse.url,
		);
		expect(getAddress).toHaveBeenCalledTimes(2);
	});

	it("signs in again on reload after sign-in fails", async () => {
		vi.spyOn(API.experimental, "getPreviewAddress").mockResolvedValue(
			MockPreviewAddressResponse,
		);
		const signIn = vi
			.spyOn(API.experimental, "signInToPreview")
			.mockRejectedValueOnce(new Error("sign-in failed"))
			.mockResolvedValue(MockPreviewSignInResponse);
		renderPreviewFrame("Docs site");
		post(
			signInNeeded,
			origin,
			previewWindow(await screen.findByTitle("Docs site")),
		);
		await screen.findByRole("alert");

		await userEvent.click(
			screen.getByRole("button", { name: "Reload preview" }),
		);
		post(
			signInNeeded,
			origin,
			previewWindow(await screen.findByTitle("Docs site")),
		);

		await waitFor(() =>
			expect(innerFrameSrc(screen.getByTitle("Docs site"))).toBe(
				MockPreviewSignInResponse.url,
			),
		);
		expect(signIn).toHaveBeenCalledTimes(2);
	});
});
