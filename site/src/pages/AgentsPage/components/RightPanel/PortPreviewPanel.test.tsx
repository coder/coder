import type { AnnotatorToHostMessage } from "@coder/annotator/protocol";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC } from "react";
import { describe, expect, it, vi } from "vitest";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	ComposerProvider,
	useRegisterComposer,
} from "../../context/ComposerContext";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import { PortPreviewPanel } from "./PortPreviewPanel";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const tab: Extract<UserRightPanelTab, { kind: "port" }> = {
	id: "port-3000",
	kind: "port",
	label: "Preview :3000",
	agentId: MockWorkspaceAgent.id,
	port: 3000,
	protocol: "http",
};

const Composer: FC<{ onSend: (message: string) => void }> = ({ onSend }) => {
	useRegisterComposer({ send: onSend });
	return null;
};

function renderPanel(onSend = vi.fn(), readyTimeoutMs?: number) {
	const view = renderComponent(
		<ComposerProvider>
			<Composer onSend={onSend} />
			<PortPreviewPanel
				workspace={MockWorkspace}
				agent={MockWorkspaceAgent}
				host="*.apps.example.com"
				tab={tab}
				canAnnotate
				annotatorReadyTimeoutMs={readyTimeoutMs}
			/>
		</ComposerProvider>,
	);
	const setAgentWorking = (isAgentWorking: boolean) =>
		view.rerender(
			<ComposerProvider>
				<Composer onSend={onSend} />
				<PortPreviewPanel
					workspace={MockWorkspace}
					agent={MockWorkspaceAgent}
					host="*.apps.example.com"
					tab={tab}
					canAnnotate
					isAgentWorking={isAgentWorking}
					annotatorReadyTimeoutMs={readyTimeoutMs}
				/>
			</ComposerProvider>,
		);
	// Requesting the overlay remounts the iframe, so always look it up fresh.
	const frame = () => screen.getByTitle<HTMLIFrameElement>("Preview :3000");
	const frameOrigin = new URL(frame().src).origin;
	const receive = (data: AnnotatorToHostMessage) => {
		window.dispatchEvent(
			new MessageEvent("message", {
				data,
				origin: frameOrigin,
				source: frame().contentWindow,
			}),
		);
	};
	return { frame, frameOrigin, receive, onSend, setAgentWorking };
}

const submission: AnnotatorToHostMessage = {
	type: "coder-annotator:submit",
	page: {
		url: "http://3000--agent--ws--user.apps.example.com/",
		title: "App",
		viewport: { width: 800, height: 600 },
	},
	annotations: [
		{
			id: "a",
			comment: "Make this red",
			element: {
				tag: "button",
				selector: "#save",
				classes: [],
				openingTag: '<button id="save">',
				rect: { x: 1, y: 2, width: 3, height: 4 },
			},
		},
	],
};

const requestOverlay = () =>
	userEvent.click(screen.getByRole("button", { name: "Annotate elements" }));

describe("PortPreviewPanel annotations", () => {
	it("requests the overlay and starts picking once it is ready", async () => {
		const { frame, frameOrigin, receive } = renderPanel();
		await requestOverlay();

		expect(new URL(frame().src).searchParams.get("coder_annotate")).toBe("1");
		const postMessage = vi.spyOn(
			frame().contentWindow as Window,
			"postMessage",
		);
		expect(postMessage).not.toHaveBeenCalled();

		receive({ type: "coder-annotator:ready" });
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true },
			frameOrigin,
		);
	});

	it("sends each submission to the agent as a message", async () => {
		const { receive, onSend } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive(submission);

		expect(onSend).toHaveBeenCalledTimes(1);
		const [message] = onSend.mock.calls[0];
		expect(message).toContain("# UI annotations");
		expect(message).toContain("> Make this red");
		expect(message).toContain("`#save`");
	});

	it("ignores the frame until the user requests the overlay", () => {
		const { receive, onSend } = renderPanel();
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		expect(onSend).not.toHaveBeenCalled();
	});

	it("drops malformed submissions", async () => {
		const { frame, frameOrigin, onSend } = renderPanel();
		await requestOverlay();
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { type: "coder-annotator:submit", page: {}, annotations: "nope" },
				origin: frameOrigin,
				source: frame().contentWindow,
			}),
		);
		expect(onSend).not.toHaveBeenCalled();
	});

	it("ignores messages from other origins", async () => {
		const { frame, onSend } = renderPanel();
		await requestOverlay();
		window.dispatchEvent(
			new MessageEvent("message", {
				data: submission,
				origin: "https://evil.example.com",
				source: frame().contentWindow,
			}),
		);
		expect(onSend).not.toHaveBeenCalled();
	});

	it("stops requesting the overlay once it failed to load", async () => {
		const { frame, receive } = renderPanel(vi.fn(), 0);
		await requestOverlay();
		const requestedSrc = frame().src;
		frame().dispatchEvent(new Event("load"));
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: "Annotate elements" }),
			).toBeDisabled(),
		);

		await userEvent.click(
			screen.getByRole("button", { name: "Annotate elements" }),
		);
		expect(frame().src).toBe(requestedSrc);

		// A late ready message recovers.
		receive({ type: "coder-annotator:ready" });
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: "Annotate elements" }),
			).toBeEnabled(),
		);
	});

	it("shimmers annotated elements while the agent works on them", async () => {
		const { frame, frameOrigin, receive, setAgentWorking } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		const postMessage = vi.spyOn(
			frame().contentWindow as Window,
			"postMessage",
		);
		await act(async () => {
			receive(submission);
		});
		expect(postMessage).not.toHaveBeenCalledWith(
			expect.objectContaining({ type: "coder-annotator:highlight" }),
			frameOrigin,
		);

		setAgentWorking(true);
		expect(postMessage).toHaveBeenCalledWith(
			{
				type: "coder-annotator:highlight",
				items: [{ id: "a", selector: "#save" }],
				state: "pending",
			},
			frameOrigin,
		);

		setAgentWorking(false);
		expect(postMessage).toHaveBeenLastCalledWith(
			{
				type: "coder-annotator:highlight",
				items: [{ id: "a", selector: "#save" }],
				state: "done",
			},
			frameOrigin,
		);
	});
});
