import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC } from "react";
import { describe, expect, it, vi } from "vitest";
import type { AnnotatorToHostMessage } from "#/annotator/protocol";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import type { AttachOptions } from "../../context/ComposerContext";
import {
	ComposerProvider,
	useRegisterComposer,
} from "../../context/ComposerContext";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import { annotationsFileName, PortPreviewPanel } from "./PortPreviewPanel";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const tab: Extract<UserRightPanelTab, { kind: "port" }> = {
	id: "port-3000",
	kind: "port",
	label: "Preview :3000",
	agentId: MockWorkspaceAgent.id,
	port: 3000,
	protocol: "http",
};

const Composer: FC<{
	onAttach: (files: File[], options?: AttachOptions) => void;
	onSend?: (message: string) => void;
}> = ({ onAttach, onSend = () => undefined }) => {
	useRegisterComposer({ attach: onAttach, send: onSend });
	return null;
};

// jsdom's Blob lacks text(); FileReader is the portable way to read it.
function readFileText(file: File): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(String(reader.result));
		reader.onerror = () => reject(reader.error);
		reader.readAsText(file);
	});
}

function renderPanel(
	onAttach = vi.fn(),
	readyTimeoutMs?: number,
	onSend = vi.fn(),
) {
	const view = renderComponent(
		<ComposerProvider>
			<Composer onAttach={onAttach} onSend={onSend} />
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
				<Composer onAttach={onAttach} />
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
	return { frame, frameOrigin, receive, onAttach, onSend, setAgentWorking };
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

	it("attaches submitted annotations to the draft as markdown", async () => {
		const { receive, onAttach } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive(submission);

		expect(onAttach).toHaveBeenCalledTimes(1);
		const [files] = onAttach.mock.calls[0];
		expect(files).toHaveLength(1);
		expect(files[0].name).toBe(annotationsFileName);
		expect(files[0].type).toBe("text/markdown");
		const text = await readFileText(files[0]);
		expect(text).toContain("> Make this red");
		expect(text).toContain("`#save`");
	});

	it("ignores the frame until the user requests the overlay", () => {
		const { receive, onAttach } = renderPanel();
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		expect(onAttach).not.toHaveBeenCalled();
	});

	it("drops malformed submissions", async () => {
		const { frame, frameOrigin, onAttach } = renderPanel();
		await requestOverlay();
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { type: "coder-annotator:submit", page: {}, annotations: "nope" },
				origin: frameOrigin,
				source: frame().contentWindow,
			}),
		);
		expect(onAttach).not.toHaveBeenCalled();
	});

	it("ignores messages from other origins", async () => {
		const { frame, onAttach } = renderPanel();
		await requestOverlay();
		window.dispatchEvent(
			new MessageEvent("message", {
				data: submission,
				origin: "https://evil.example.com",
				source: frame().contentWindow,
			}),
		);
		expect(onAttach).not.toHaveBeenCalled();
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
		const { frame, frameOrigin, receive, onAttach, setAgentWorking } =
			renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		const postMessage = vi.spyOn(
			frame().contentWindow as Window,
			"postMessage",
		);

		const [, options] = onAttach.mock.calls[0] as [File[], AttachOptions];
		act(() => options.onSent?.());
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

	it("sends instant submissions as a message instead of attaching", async () => {
		const { receive, onAttach, onSend } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive({ ...submission, instant: true });

		expect(onAttach).not.toHaveBeenCalled();
		expect(onSend).toHaveBeenCalledTimes(1);
		expect(onSend.mock.calls[0][0]).toContain("> Make this red");
	});
});
