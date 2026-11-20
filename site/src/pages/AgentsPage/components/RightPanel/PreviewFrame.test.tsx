import { render, screen } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type {
	PreviewSignInRequest,
	PreviewSignInResponse,
} from "#/api/typesGenerated";
import { getPreferredProxy, ProxyContext } from "#/contexts/ProxyContext";
import { MockPreviewSignInResponse } from "#/testHelpers/chatEntities";
import { MockHealthyWildWorkspaceProxy } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { PreviewFrame } from "./PreviewFrame";

const mockPreviewSignInRequest: Omit<PreviewSignInRequest, "region_id"> = {
	workspace_id: "workspace-1",
	agent_name: "main",
	preview_slug: "preview--3000",
	url_path: "/docs",
};

const { origin } = MockPreviewSignInResponse;

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
				<PreviewFrame preview={mockPreviewSignInRequest} title={title} />
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

afterEach(() => {
	vi.restoreAllMocks();
});

describe("PreviewFrame", () => {
	it("signs in through the selected proxy and frames only the preview's origin, in the server's sandbox", async () => {
		const mockSignIn: PreviewSignInResponse = {
			...MockPreviewSignInResponse,
			url: `${origin}/docs?a="1"&b=<x>&coder_preview_sign_in=token`,
		};
		const title = '"><script>document.title="x"</script>';
		const signIn = vi
			.spyOn(API.experimental, "signInToPreview")
			.mockResolvedValue(mockSignIn);
		renderPreviewFrame(title);

		const frame = await screen.findByTitle(title);
		const wrapper = parseWrapper(frame);
		const innerFrames = wrapper.querySelectorAll("iframe");

		expect(signIn).toHaveBeenCalledWith({
			...mockPreviewSignInRequest,
			region_id: MockHealthyWildWorkspaceProxy.id,
		});
		expect(frame.getAttribute("sandbox")).toBe(mockSignIn.sandbox);
		expect(
			wrapper
				.querySelector('meta[http-equiv="Content-Security-Policy"]')
				?.getAttribute("content"),
		).toBe(
			`default-src 'none'; style-src 'unsafe-inline'; frame-src ${origin}`,
		);
		expect(wrapper.querySelectorAll("script")).toHaveLength(0);
		expect(innerFrames).toHaveLength(1);
		expect(innerFrames[0].getAttribute("src")).toBe(mockSignIn.url);
		expect(innerFrames[0].getAttribute("title")).toBe(title);
		expect(innerFrames[0].getAttribute("referrerpolicy")).toBe("no-referrer");
	});
});
