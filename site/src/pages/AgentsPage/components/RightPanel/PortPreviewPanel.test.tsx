import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC } from "react";
import { describe, expect, it, vi } from "vitest";
import type { AnnotatorToHostMessage } from "#/annotator/protocol";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	ComposerAttachmentsProvider,
	useRegisterComposerAttachments,
} from "../../context/ComposerAttachmentsContext";
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

const Composer: FC<{ onAttach: (files: File[]) => void }> = ({ onAttach }) => {
	useRegisterComposerAttachments(onAttach);
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

function renderPanel(onAttach = vi.fn()) {
	renderComponent(
		<ComposerAttachmentsProvider>
			<Composer onAttach={onAttach} />
			<PortPreviewPanel
				workspace={MockWorkspace}
				agent={MockWorkspaceAgent}
				host="*.apps.example.com"
				tab={tab}
				canAnnotate
			/>
		</ComposerAttachmentsProvider>,
	);
	const frame = screen.getByTitle<HTMLIFrameElement>("Preview :3000");
	const frameOrigin = new URL(frame.src).origin;
	const receive = (data: AnnotatorToHostMessage) => {
		window.dispatchEvent(
			new MessageEvent("message", {
				data,
				origin: frameOrigin,
				source: frame.contentWindow,
			}),
		);
	};
	return { frame, frameOrigin, receive, onAttach };
}

describe("PortPreviewPanel annotations", () => {
	it("requests the overlay and starts picking once it is ready", async () => {
		const { frame, frameOrigin, receive } = renderPanel();
		expect(new URL(frame.src).searchParams.has("coder_annotate")).toBe(false);

		await userEvent.click(
			screen.getByRole("button", { name: "Annotate elements" }),
		);

		expect(new URL(frame.src).searchParams.get("coder_annotate")).toBe("1");
		// jsdom swaps the iframe's window when src changes, so spy on the
		// window that will receive the ready message.
		const postMessage = vi.spyOn(frame.contentWindow as Window, "postMessage");
		expect(postMessage).not.toHaveBeenCalled();

		receive({ type: "coder-annotator:ready" });
		expect(postMessage).toHaveBeenCalledWith(
			{ type: "coder-annotator:set-picking", picking: true },
			frameOrigin,
		);
	});

	it("attaches submitted annotations to the composer as markdown", async () => {
		const { receive, onAttach } = renderPanel();
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

		expect(onAttach).toHaveBeenCalledTimes(1);
		const [files] = onAttach.mock.calls[0];
		expect(files).toHaveLength(1);
		expect(files[0].name).toBe(annotationsFileName);
		expect(files[0].type).toBe("text/markdown");
		await expect(readFileText(files[0])).resolves.toContain(
			"## 1. Make this red",
		);
	});

	it("ignores messages from other origins", () => {
		const { frame, onAttach } = renderPanel();
		window.dispatchEvent(
			new MessageEvent("message", {
				data: {
					type: "coder-annotator:submit",
					page: { url: "", title: "", viewport: { width: 0, height: 0 } },
					annotations: [],
				},
				origin: "https://evil.example.com",
				source: frame.contentWindow,
			}),
		);
		expect(onAttach).not.toHaveBeenCalled();
	});

	it("hides the annotate control without the experiment", () => {
		renderComponent(
			<ComposerAttachmentsProvider>
				<Composer onAttach={vi.fn()} />
				<PortPreviewPanel
					workspace={MockWorkspace}
					agent={MockWorkspaceAgent}
					host="*.apps.example.com"
					tab={tab}
				/>
			</ComposerAttachmentsProvider>,
		);
		expect(
			screen.queryByRole("button", { name: "Annotate elements" }),
		).toBeNull();
	});
});
