import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC } from "react";
import { describe, expect, it, vi } from "vitest";
import type { AnnotatorToHostMessage } from "#/annotator/protocol";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	ComposerProvider,
	type ComposerSendResult,
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

type Send = (message: string) => Promise<ComposerSendResult>;

const Composer: FC<{ onSend: Send }> = ({ onSend }) => {
	useRegisterComposer({ send: onSend });
	return null;
};

const sent = () => vi.fn<Send>().mockResolvedValue("sent");

function frameWindow(frame: HTMLIFrameElement): Window {
	if (!frame.contentWindow) {
		throw new Error("frame has no window");
	}
	return frame.contentWindow;
}

function renderPanel(onSend = sent(), readyTimeoutMs?: number) {
	const panel = (isAgentWorking: boolean) => (
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
		</ComposerProvider>
	);
	const view = renderComponent(panel(false));
	const setAgentWorking = (isAgentWorking: boolean) =>
		view.rerender(panel(isAgentWorking));
	// Requesting the overlay remounts the iframe, so always look it up fresh.
	const frame = () => screen.getByTitle<HTMLIFrameElement>("Preview :3000");
	const frameOrigin = new URL(frame().src).origin;
	const receive = (data: AnnotatorToHostMessage) => {
		window.dispatchEvent(
			new MessageEvent("message", {
				data,
				origin: frameOrigin,
				source: frameWindow(frame()),
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

const annotateButton = () =>
	screen.getByRole("button", { name: /annotate elements|stop annotating/i });

const requestOverlay = () => userEvent.click(annotateButton());

describe("PortPreviewPanel annotations", () => {
	it("requests the overlay and starts picking once it is ready", async () => {
		const { frame, frameOrigin, receive } = renderPanel();
		await requestOverlay();

		expect(new URL(frame().src).searchParams.get("coder_annotate")).toBe("1");
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		expect(postMessage).not.toHaveBeenCalled();

		receive({ type: "coder-annotator:ready" });
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true, hint: true },
			frameOrigin,
		);
	});

	it("does not reload the frame for clicks while the overlay is loading", async () => {
		const { frame } = renderPanel();
		await requestOverlay();
		const requestedSrc = frame().src;
		const requestedFrame = frame();

		await userEvent.click(annotateButton());
		expect(frame()).toBe(requestedFrame);
		expect(frame().src).toBe(requestedSrc);
	});

	it("lets a click while the overlay is loading cancel the request", async () => {
		const { frame, frameOrigin, receive } = renderPanel();
		await requestOverlay();
		frame().dispatchEvent(new Event("load"));
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");

		// Second click: the user changed their mind before the overlay loaded.
		await userEvent.click(annotateButton());
		receive({ type: "coder-annotator:ready" });
		expect(postMessage).not.toHaveBeenCalled();

		// Asking again after the overlay is ready no longer needs a reload.
		const readyFrame = frame();
		await userEvent.click(annotateButton());
		expect(frame()).toBe(readyFrame);
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true, hint: true },
			frameOrigin,
		);
	});

	it("keeps the request through the overlay's initial state reports", async () => {
		const { receive, onSend } = renderPanel();
		await requestOverlay();
		// The overlay reports idle when it mounts and again after ready, before
		// it has processed the dashboard's request to pick.
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: false });
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: false });
			receive({ type: "coder-annotator:state", picking: true });
		});
		expect(annotateButton()).toHaveAttribute("aria-pressed", "true");

		receive(submission);
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
	});

	it("leaves annotate mode on Escape pressed in the dashboard", async () => {
		const { frame, frameOrigin, receive } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		const focus = vi.spyOn(frame(), "focus");
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: true });
		});
		expect(focus).toHaveBeenCalled();

		await userEvent.keyboard("{Escape}");
		expect(postMessage).toHaveBeenLastCalledWith(
			{ type: "coder-annotator:set-picking", picking: false, hint: true },
			frameOrigin,
		);
	});

	it("sends each submission to the agent as a message", async () => {
		const { receive, onSend } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive(submission);

		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		const [message] = onSend.mock.calls[0];
		expect(message).toContain("# UI annotations");
		expect(message).toContain("> Make this red");
		expect(message).toContain("`#save`");
	});

	it("sends submissions in order and retries while the chat is busy", async () => {
		const onSend = vi
			.fn<Send>()
			.mockResolvedValueOnce("busy")
			.mockResolvedValue("sent");
		const { receive } = renderPanel(onSend);
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		receive({
			...submission,
			annotations: [
				{ ...submission.annotations[0], id: "b", comment: "Then this" },
			],
		});

		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(3), {
			timeout: 3000,
		});
		expect(onSend.mock.calls[0][0]).toContain("> Make this red");
		expect(onSend.mock.calls[1][0]).toContain("> Make this red");
		expect(onSend.mock.calls[2][0]).toContain("> Then this");
	});

	it("stops asking for the first-run hint once it was dismissed", async () => {
		window.localStorage.removeItem("coder.annotator.hint-dismissed");
		const { frame, frameOrigin, receive } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		receive({ type: "coder-annotator:hint-dismissed" });
		expect(window.localStorage.getItem("coder.annotator.hint-dismissed")).toBe(
			"1",
		);

		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: true });
		});
		await userEvent.keyboard("{Escape}");
		expect(postMessage).toHaveBeenLastCalledWith(
			{ type: "coder-annotator:set-picking", picking: false, hint: false },
			frameOrigin,
		);
		window.localStorage.removeItem("coder.annotator.hint-dismissed");
	});

	it("ignores the frame until the user requests the overlay", () => {
		const { receive, onSend } = renderPanel();
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		expect(onSend).not.toHaveBeenCalled();
	});

	it("ignores submissions once the user has left annotate mode", async () => {
		const { receive, onSend } = renderPanel();
		await requestOverlay();
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: true });
			// The user switches picking off from inside the preview; anything
			// the page posts afterwards is not an annotation the user made.
			receive({ type: "coder-annotator:state", picking: false });
		});
		expect(annotateButton()).toHaveAttribute("aria-pressed", "false");
		receive(submission);
		await new Promise((resolve) => setTimeout(resolve, 20));
		expect(onSend).not.toHaveBeenCalled();
	});

	it("refuses picking the dashboard did not start", async () => {
		const { frame, frameOrigin, receive, onSend } = renderPanel();
		await requestOverlay();
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: true });
		});
		await userEvent.keyboard("{Escape}");
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: false });
		});
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");

		// The page claims annotate mode is back on without the user asking.
		await act(async () => {
			receive({ type: "coder-annotator:state", picking: true });
		});
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: false },
			frameOrigin,
		);
		expect(annotateButton()).toHaveAttribute("aria-pressed", "false");
		receive(submission);
		await new Promise((resolve) => setTimeout(resolve, 20));
		expect(onSend).not.toHaveBeenCalled();
	});

	it("disarms when the frame loads another document", async () => {
		const { frame, receive, onSend } = renderPanel();
		await requestOverlay();
		await act(async () => {
			receive({ type: "coder-annotator:ready" });
			receive({ type: "coder-annotator:state", picking: true });
		});

		// The app navigated on its own. Whatever the new document sends, it
		// was never told to pick.
		await act(async () => {
			frame().dispatchEvent(new Event("load"));
		});
		expect(annotateButton()).toHaveAttribute("aria-pressed", "false");
		receive({ type: "coder-annotator:ready" });
		receive(submission);
		await new Promise((resolve) => setTimeout(resolve, 20));
		expect(onSend).not.toHaveBeenCalled();
	});

	it("requests the overlay again after the app navigated away from it", async () => {
		const { frame, frameOrigin, receive } = renderPanel(sent(), 0);
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		const loadedFrame = frame();

		// A navigation without the marker drops the overlay. That is not a
		// failure to load, so the control must not give up.
		await act(async () => {
			loadedFrame.dispatchEvent(new Event("load"));
			await new Promise((resolve) => setTimeout(resolve, 10));
		});
		expect(annotateButton()).not.toHaveAttribute("aria-disabled", "true");
		expect(annotateButton()).not.toHaveAttribute("aria-busy", "true");

		await userEvent.click(annotateButton());
		expect(frame()).not.toBe(loadedFrame);
		expect(new URL(frame().src).searchParams.get("coder_annotate")).toBe("1");
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		receive({ type: "coder-annotator:ready" });
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true, hint: true },
			frameOrigin,
		);
	});

	it("drops malformed submissions", async () => {
		const { frame, frameOrigin, onSend } = renderPanel();
		await requestOverlay();
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { type: "coder-annotator:submit", page: {}, annotations: "nope" },
				origin: frameOrigin,
				source: frameWindow(frame()),
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
				source: frameWindow(frame()),
			}),
		);
		expect(onSend).not.toHaveBeenCalled();
	});

	it("shimmers annotated elements while the agent works on them", async () => {
		const { frame, frameOrigin, receive, onSend, setAgentWorking } =
			renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		receive(submission);
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		expect(postMessage).not.toHaveBeenCalledWith(
			expect.objectContaining({ type: "coder-annotator:highlight" }),
			frameOrigin,
		);

		setAgentWorking(true);
		const item = {
			id: "a",
			selector: "#save",
			url: "http://3000--agent--ws--user.apps.example.com/",
		};
		await waitFor(() =>
			expect(postMessage).toHaveBeenCalledWith(
				{ type: "coder-annotator:highlight", items: [item] },
				frameOrigin,
			),
		);

		// A second annotation during the same turn joins the first.
		receive({
			...submission,
			annotations: [{ ...submission.annotations[0], id: "b" }],
		});
		await waitFor(() =>
			expect(postMessage).toHaveBeenCalledWith(
				{
					type: "coder-annotator:highlight",
					items: [item, { ...item, id: "b" }],
				},
				frameOrigin,
			),
		);

		setAgentWorking(false);
		await waitFor(() =>
			expect(postMessage).toHaveBeenLastCalledWith(
				{ type: "coder-annotator:clear-highlights" },
				frameOrigin,
			),
		);
	});

	it("hands annotate mode to a popout opened while the frame is ready", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		const { frame, frameOrigin, receive, onSend } = renderPanel();
		await requestOverlay();
		receive({ type: "coder-annotator:ready" });
		const framePost = vi.spyOn(frameWindow(frame()), "postMessage");

		// A stand-in popout window. jsdom never sets `closed`, so it is
		// defined here and flipped by hand below.
		const holder = document.createElement("iframe");
		document.body.appendChild(holder);
		const popoutWindow = frameWindow(holder);
		let closed = false;
		Object.defineProperty(popoutWindow, "closed", { get: () => closed });
		const open = vi.spyOn(window, "open").mockReturnValue(popoutWindow);
		const popoutPost = vi.spyOn(popoutWindow, "postMessage");

		await userEvent.click(
			screen.getByRole("button", { name: "Open port in new tab" }),
		);
		const [openedUrl] = open.mock.calls[0];
		expect(new URL(String(openedUrl)).searchParams.get("coder_annotate")).toBe(
			"1",
		);
		// The frame's overlay is switched off, not left picking alongside.
		expect(framePost).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: false, hint: true },
			frameOrigin,
		);
		expect(framePost).not.toHaveBeenCalledWith(
			expect.objectContaining({ picking: true }),
			frameOrigin,
		);

		const fromPopout = (data: AnnotatorToHostMessage) =>
			window.dispatchEvent(
				new MessageEvent("message", {
					data,
					origin: frameOrigin,
					source: popoutWindow,
				}),
			);
		fromPopout({ type: "coder-annotator:ready" });
		expect(popoutPost).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true, hint: true },
			frameOrigin,
		);
		fromPopout(submission);
		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));

		// Closing hands control back; the popout can no longer submit, and
		// the panel is idle rather than waiting on an overlay that will not
		// announce itself again. Asking again reloads the frame.
		closed = true;
		await act(async () => {
			await vi.advanceTimersByTimeAsync(600);
		});
		fromPopout(submission);
		await act(async () => {
			await vi.advanceTimersByTimeAsync(50);
		});
		expect(onSend).toHaveBeenCalledTimes(1);
		expect(annotateButton()).not.toHaveAttribute("aria-busy", "true");
		const idleFrame = frame();
		await userEvent.click(annotateButton());
		expect(frame()).not.toBe(idleFrame);
		vi.useRealTimers();
		open.mockRestore();
		holder.remove();
	});

	it("gives up on a popout whose overlay never reports in", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		renderPanel(sent(), 100);
		const holder = document.createElement("iframe");
		document.body.appendChild(holder);
		const popoutWindow = frameWindow(holder);
		Object.defineProperty(popoutWindow, "closed", { get: () => false });
		const open = vi.spyOn(window, "open").mockReturnValue(popoutWindow);

		await userEvent.click(
			screen.getByRole("button", { name: "Open port in new tab" }),
		);
		expect(
			screen.getByRole("button", { name: "Open port in new tab" }),
		).toHaveAttribute("aria-pressed", "true");
		await act(async () => {
			await vi.advanceTimersByTimeAsync(200);
		});
		expect(
			screen.getByRole("button", { name: "Open port in new tab" }),
		).toHaveAttribute("aria-pressed", "false");
		vi.useRealTimers();
		open.mockRestore();
		holder.remove();
	});

	it("stops requesting the overlay once it failed to load", async () => {
		const { frame, frameOrigin, receive } = renderPanel(sent(), 0);
		await requestOverlay();
		const requestedSrc = frame().src;
		const requestedFrame = frame();
		frame().dispatchEvent(new Event("load"));
		await waitFor(() =>
			expect(annotateButton()).toHaveAttribute("aria-disabled", "true"),
		);
		// Still focusable, so keyboard users get the explanation too.
		expect(annotateButton()).toHaveAccessibleDescription(
			/could not load in this app/,
		);

		await userEvent.click(annotateButton());
		expect(annotateButton()).toHaveFocus();
		await userEvent.keyboard("{Enter} ");
		expect(frame()).toBe(requestedFrame);
		expect(frame().src).toBe(requestedSrc);

		// A late ready message recovers and delivers the pending request.
		const postMessage = vi.spyOn(frameWindow(frame()), "postMessage");
		receive({ type: "coder-annotator:ready" });
		await waitFor(() =>
			expect(postMessage).toHaveBeenCalledWith(
				{ type: "coder-annotator:set-picking", picking: true, hint: true },
				frameOrigin,
			),
		);
		expect(annotateButton()).not.toHaveAttribute("aria-disabled", "true");
	});
});
