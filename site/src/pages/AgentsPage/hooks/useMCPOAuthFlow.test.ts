import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { useMCPOAuthFlow } from "./useMCPOAuthFlow";

const dispatchComplete = (
	serverID: string,
	source: MessageEventSource | null,
) => {
	act(() => {
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { type: "mcp-oauth2-complete", serverID },
				origin: location.origin,
				source,
			}),
		);
	});
};

// An iframe's window stands in for the popup so the hook sees a real
// Window. jsdom does not implement Window.closed, so closing defines it.
const createPopup = () => {
	const iframe = document.createElement("iframe");
	document.body.appendChild(iframe);
	const popup = iframe.contentWindow;
	if (!popup) {
		throw new Error("iframe contentWindow unavailable");
	}
	return {
		popup,
		closePopup: () => Object.defineProperty(popup, "closed", { value: true }),
	};
};

const renderFlow = () => {
	const onAuthComplete = vi.fn();
	const onFlowSuccess = vi.fn();
	const hook = renderHook(() =>
		useMCPOAuthFlow({ organizationId: "org-1", onAuthComplete, onFlowSuccess }),
	);
	return { ...hook, onAuthComplete, onFlowSuccess };
};

describe("useMCPOAuthFlow", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("opens the organization's connect path and reports the initiating popup's completion", () => {
		const { popup } = createPopup();
		const open = vi.spyOn(window, "open").mockReturnValue(popup);
		const { result, onAuthComplete, onFlowSuccess } = renderFlow();

		act(() => result.current.connect(MockMCPServerConfig));
		expect(open).toHaveBeenCalledWith(
			`/api/v2/organizations/org-1/mcp-servers/${MockMCPServerConfig.id}/oauth2/connect`,
			"_blank",
			"width=900,height=600",
		);
		expect(result.current.connectingServerId).toBe(MockMCPServerConfig.id);

		dispatchComplete(MockMCPServerConfig.id, popup);
		expect(onAuthComplete).toHaveBeenCalledWith(MockMCPServerConfig.id);
		expect(onFlowSuccess).toHaveBeenCalledWith(MockMCPServerConfig);
		expect(result.current.connectingServerId).toBeNull();
	});

	it("does not enter the connecting state when the popup is blocked", () => {
		vi.spyOn(window, "open").mockReturnValue(null);
		const { result } = renderFlow();

		act(() => result.current.connect(MockMCPServerConfig));
		expect(result.current.connectingServerId).toBeNull();
	});

	it("refreshes but does not report success for an unsolicited completion", () => {
		const { onAuthComplete, onFlowSuccess } = renderFlow();

		dispatchComplete("mcp-github", window);
		expect(onAuthComplete).toHaveBeenCalledWith("mcp-github");
		expect(onFlowSuccess).not.toHaveBeenCalled();
	});

	it("does not report success for a completion about another server", () => {
		const { popup } = createPopup();
		vi.spyOn(window, "open").mockReturnValue(popup);
		const { result, onAuthComplete, onFlowSuccess } = renderFlow();

		act(() => result.current.connect(MockMCPServerConfig));
		dispatchComplete("mcp-linear", popup);
		expect(onAuthComplete).toHaveBeenCalledWith("mcp-linear");
		expect(onFlowSuccess).not.toHaveBeenCalled();
	});

	it("reports success when the popup closes before its completion message arrives", async () => {
		const { popup, closePopup } = createPopup();
		vi.spyOn(window, "open").mockReturnValue(popup);
		const { result, onFlowSuccess } = renderFlow();

		act(() => result.current.connect(MockMCPServerConfig));
		closePopup();
		await waitFor(() => expect(result.current.connectingServerId).toBeNull(), {
			timeout: 2_000,
		});
		dispatchComplete(MockMCPServerConfig.id, popup);
		expect(onFlowSuccess).toHaveBeenCalledWith(MockMCPServerConfig);
	});
});
