import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC } from "react";
import { describe, expect, it, vi } from "vitest";
import type { AnnotatorToHostMessage } from "#/annotator/protocol";
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

function renderPanel(onSend = vi.fn()) {
	renderComponent(
		<ComposerProvider>
			<Composer onSend={onSend} />
			<PortPreviewPanel
				workspace={MockWorkspace}
				agent={MockWorkspaceAgent}
				host="*.apps.example.com"
				tab={tab}
				canAnnotate
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
	return { frame, frameOrigin, receive, onSend };
}

describe("PortPreviewPanel annotations", () => {
	it("requests the overlay and starts picking once it is ready", async () => {
		const { frame, frameOrigin, receive } = renderPanel();
		expect(new URL(frame().src).searchParams.has("coder_annotate")).toBe(false);

		await userEvent.click(
			screen.getByRole("button", { name: "Annotate elements" }),
		);

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

	it("sends submitted annotations as a chat message", () => {
		const { receive, onSend } = renderPanel();
		receive({ type: "coder-annotator:ready" });
		receive({
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
						html: '<button id="save">Save</button>',
						rect: { x: 1, y: 2, width: 3, height: 4 },
					},
				},
			],
		});

		expect(onSend).toHaveBeenCalledTimes(1);
		const [message] = onSend.mock.calls[0];
		expect(message).toContain("# UI annotations");
		expect(message).toContain("## 1. Make this red");
		expect(message).toContain("`#save`");
	});

	it("ignores messages from other origins", () => {
		const { frame, onSend } = renderPanel();
		window.dispatchEvent(
			new MessageEvent("message", {
				data: {
					type: "coder-annotator:submit",
					page: { url: "", title: "", viewport: { width: 0, height: 0 } },
					annotations: [],
				},
				origin: "https://evil.example.com",
				source: frame().contentWindow,
			}),
		);
		expect(onSend).not.toHaveBeenCalled();
	});

	it("hides the annotate control without the experiment", () => {
		renderComponent(
			<ComposerProvider>
				<Composer onSend={vi.fn()} />
				<PortPreviewPanel
					workspace={MockWorkspace}
					agent={MockWorkspaceAgent}
					host="*.apps.example.com"
					tab={tab}
				/>
			</ComposerProvider>,
		);
		expect(
			screen.queryByRole("button", { name: "Annotate elements" }),
		).toBeNull();
	});
});
