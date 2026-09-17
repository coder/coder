import { fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import type { ChatWindow } from "./boardStorage";
import { FloatingChat } from "./ChatWindows";

vi.mock("../../AgentChatPage", () => ({
	default: () => <div>chat body</div>,
}));

const win: ChatWindow = {
	chatId: MockChat.id,
	x: 100,
	y: 80,
	width: 400,
	height: 300,
	pinned: true,
};

const renderWindow = () => {
	const onChange = vi.fn();
	renderComponent(
		<FloatingChat
			window={win}
			chat={MockChat}
			color={undefined}
			onChange={onChange}
			onClose={vi.fn()}
			onInteract={vi.fn()}
			onPreviewEnter={vi.fn()}
			onPreviewLeave={vi.fn()}
		/>,
	);
	return { onChange };
};

const drag = (
	handle: Element,
	from: { x: number; y: number },
	to: { x: number; y: number },
) => {
	fireEvent.pointerDown(handle, {
		button: 0,
		clientX: from.x,
		clientY: from.y,
	});
	fireEvent.pointerMove(window, { clientX: to.x, clientY: to.y });
	fireEvent.pointerUp(window, { clientX: to.x, clientY: to.y });
};

describe("FloatingChat", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("moves by the title bar and commits once on release", () => {
		const { onChange } = renderWindow();
		drag(
			screen.getByText(MockChat.title),
			{ x: 150, y: 90 },
			{ x: 180, y: 130 },
		);
		expect(onChange).toHaveBeenCalledTimes(1);
		expect(onChange).toHaveBeenCalledWith({ ...win, x: 130, y: 120 });
	});

	it("resizes by the corner handle", () => {
		const { onChange } = renderWindow();
		const corner = screen.getByRole("presentation", { hidden: true });
		drag(corner, { x: 500, y: 380 }, { x: 560, y: 400 });
		expect(onChange).toHaveBeenCalledWith({ ...win, width: 460, height: 320 });
	});

	it("ignores a press without movement", () => {
		const { onChange } = renderWindow();
		const title = screen.getByText(MockChat.title);
		fireEvent.pointerDown(title, { button: 0, clientX: 150, clientY: 90 });
		fireEvent.pointerUp(window, { clientX: 150, clientY: 90 });
		expect(onChange).not.toHaveBeenCalled();
	});

	it("stops listening on the window after the gesture ends", () => {
		const { onChange } = renderWindow();
		drag(
			screen.getByText(MockChat.title),
			{ x: 150, y: 90 },
			{ x: 160, y: 90 },
		);
		fireEvent.pointerMove(window, { clientX: 400, clientY: 400 });
		fireEvent.pointerUp(window, { clientX: 400, clientY: 400 });
		expect(onChange).toHaveBeenCalledTimes(1);
	});

	it("draws one frame per burst of moves and commits the last position", () => {
		// Frames never run: the release must still commit the latest geometry,
		// and the frame left pending must be cancelled with the listeners.
		const raf = vi
			.spyOn(window, "requestAnimationFrame")
			.mockImplementation(() => 7);
		const caf = vi
			.spyOn(window, "cancelAnimationFrame")
			.mockImplementation(() => undefined);
		const { onChange } = renderWindow();
		const title = screen.getByText(MockChat.title);
		fireEvent.pointerDown(title, { button: 0, clientX: 150, clientY: 90 });
		fireEvent.pointerMove(window, { clientX: 160, clientY: 100 });
		fireEvent.pointerMove(window, { clientX: 170, clientY: 110 });
		fireEvent.pointerMove(window, { clientX: 180, clientY: 130 });
		expect(raf).toHaveBeenCalledTimes(1);
		fireEvent.pointerUp(window, { clientX: 180, clientY: 130 });
		expect(caf).toHaveBeenCalledWith(7);
		expect(onChange).toHaveBeenCalledTimes(1);
		expect(onChange).toHaveBeenCalledWith({ ...win, x: 130, y: 120 });
	});
});
