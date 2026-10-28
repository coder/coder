import { render, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { LazyTerminalPanel } from "./LazyTerminalPanel";

const panel = vi.hoisted(() => ({ loads: 0, mounts: 0, unmounts: 0 }));

vi.mock("./TerminalPanel", async () => {
	panel.loads++;
	const { useEffect } = await import("react");
	const MockTerminalPanel = () => {
		useEffect(() => {
			panel.mounts++;
			return () => {
				panel.unmounts++;
			};
		}, []);
		return null;
	};
	return { default: MockTerminalPanel };
});

const props = {
	chatId: "chat-1",
	autoFocus: false,
	onReady: () => {},
	workspace: MockWorkspace,
	workspaceAgent: MockWorkspaceAgent,
};

describe("LazyTerminalPanel", () => {
	it("loads the panel when its tab is first hot and keeps it mounted after", async () => {
		const { rerender } = render(<LazyTerminalPanel {...props} isHot={false} />);
		await vi.dynamicImportSettled();
		expect(panel.loads).toBe(0);

		rerender(<LazyTerminalPanel {...props} isHot />);
		await waitFor(() => expect(panel.mounts).toBe(1));
		expect(panel.loads).toBe(1);

		rerender(<LazyTerminalPanel {...props} isHot={false} />);
		expect(panel.unmounts).toBe(0);
	});

	it("signals ready while the chunk loads and leaves it to the panel after", async () => {
		// A fresh module, since the first test already loaded the chunk.
		vi.resetModules();
		const fresh = await import("./LazyTerminalPanel");
		const onReady = vi.fn();
		const mounts = panel.mounts;
		const first = render(
			<fresh.LazyTerminalPanel {...props} isHot onReady={onReady} />,
		);
		expect(onReady).toHaveBeenCalledTimes(1);
		await waitFor(() => expect(panel.mounts).toBe(mounts + 1));
		first.unmount();

		render(<fresh.LazyTerminalPanel {...props} isHot onReady={onReady} />);
		expect(panel.mounts).toBe(mounts + 2);
		expect(onReady).toHaveBeenCalledTimes(1);
	});
});
