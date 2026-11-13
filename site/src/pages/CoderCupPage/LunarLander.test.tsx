import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LunarLander } from "./LunarLander";

vi.mock("./roster", () => ({
	ROSTER: [
		{ name: "Nova Moonfern", role: "Community Member", isCommunity: true },
		{ name: "Employee", role: "Community Team Lead" },
	],
}));

vi.stubGlobal("jest", vi);
await import("jest-canvas-mock");

afterEach(() => {
	vi.restoreAllMocks();
});

function renderGame() {
	vi.spyOn(Math, "random").mockReturnValue(0.5);
	vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
		new DOMRect(0, 0, 1200, 800),
	);
	const frames: FrameRequestCallback[] = [];
	vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
		frames.push(callback);
		return frames.length;
	});
	vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => {});
	const context = document.createElement("canvas").getContext("2d");
	if (!context) throw new Error("Canvas context unavailable");
	vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(context);
	const fillText = vi.spyOn(context, "fillText");

	const user = userEvent.setup();
	const result = render(<LunarLander />);
	const runFrame = (timestamp: number) => {
		const frame = frames.shift();
		if (!frame) throw new Error("Animation frame not scheduled");
		act(() => frame(timestamp));
	};
	const sidebarDrawn = () =>
		fillText.mock.calls.some(([text]) => text === "Employee");
	return { ...result, context, fillText, runFrame, sidebarDrawn, user };
}

it("carries community membership from the roster into sidebar and suit drawing", async () => {
	const { context, fillText, runFrame, unmount, user } = renderGame();
	const suitColors = new Set<string | CanvasGradient | CanvasPattern>();
	vi.spyOn(context, "stroke").mockImplementation(() => {
		suitColors.add(context.strokeStyle);
	});

	runFrame(1);
	await user.keyboard("h");
	runFrame(17);

	expect(fillText).toHaveBeenCalledWith(
		"Nova Moonfern (community)",
		expect.any(Number),
		expect.any(Number),
	);
	expect(fillText).toHaveBeenCalledWith(
		"Employee",
		expect.any(Number),
		expect.any(Number),
	);
	expect(suitColors).toContain("#666666");
	expect(suitColors).toContain("#888888");
	unmount();
});

it("waits on the start screen until a difficulty button is clicked", async () => {
	const { runFrame, sidebarDrawn, unmount, user } = renderGame();

	// The start screen stays up until a difficulty is chosen.
	runFrame(1);
	runFrame(60_000);
	expect(sidebarDrawn()).toBe(false);

	await user.click(screen.getByRole("button", { name: "Easy (E)" }));
	runFrame(60_017);
	expect(sidebarDrawn()).toBe(true);
	expect(screen.queryByRole("button", { name: /easy/i })).toBeNull();
	unmount();
});

it("starts the game with the uppercase keyboard shortcut", async () => {
	const { runFrame, sidebarDrawn, unmount, user } = renderGame();
	runFrame(1);
	await user.keyboard("M");
	runFrame(17);
	expect(sidebarDrawn()).toBe(true);
	expect(screen.queryByRole("button", { name: /moderate/i })).toBeNull();
	unmount();
});

it.each([
	{ key: "e", warns: false },
	{ key: "m", warns: false },
	{ key: "h", warns: true },
])(
	"velocity warning follows the difficulty chosen with $key",
	async ({ key, warns }) => {
		const { fillText, runFrame, unmount, user } = renderGame();
		runFrame(1);
		await user.keyboard(key);
		runFrame(2);
		fillText.mockClear();

		// Two seconds of free fall at 10 px/s^2 reaches 20 px/s: above the
		// Hard limit (15) and below Moderate (30) and Easy (50).
		runFrame(2002);

		const warned = fillText.mock.calls.some(([text]) => text === "⚠ WARNING");
		expect(warned).toBe(warns);
		unmount();
	},
);

it("rewinds recent play behind a REWINDING... label when R is pressed", async () => {
	const { fillText, runFrame, unmount, user } = renderGame();
	const drew = (text: string) =>
		fillText.mock.calls.some(([drawn]) => drawn === text);
	let now = 1;
	const advance = (ms: number) => {
		now += ms;
		runFrame(now);
	};

	// Two seconds of free fall on Hard trips the velocity warning.
	runFrame(now);
	await user.keyboard("h");
	advance(1);
	for (let i = 0; i < 20; i++) advance(100);
	fillText.mockClear();
	advance(16);
	expect(drew("⚠ WARNING")).toBe(true);

	await user.keyboard("R");
	fillText.mockClear();
	advance(100);
	expect(drew("REWINDING...")).toBe(true);
	expect(screen.getByText("Rewinding 5 seconds")).toBeInTheDocument();

	// The replay runs faster than real time and covers all of the
	// play so far, so one second is enough to finish it.
	for (let i = 0; i < 10; i++) advance(100);
	fillText.mockClear();
	advance(16);
	expect(drew("REWINDING...")).toBe(false);
	expect(drew("⚠ WARNING")).toBe(false);
	expect(screen.queryByText("Rewinding 5 seconds")).toBeNull();
	unmount();
});

it("brings a crashed lander back when R is pressed during the explosion", async () => {
	const { context, runFrame, unmount, user } = renderGame();
	// drawLander is the only caller of rotate, so a frame without one is
	// a frame where the lander has been replaced by its explosion.
	const rotate = vi.spyOn(context, "rotate");
	let now = 1;
	const advance = (ms: number) => {
		now += ms;
		rotate.mockClear();
		runFrame(now);
		return rotate.mock.calls.length > 0;
	};

	runFrame(now);
	await user.keyboard("h");
	let crashed = false;
	for (let i = 0; i < 100 && !crashed; i++) crashed = !advance(100);
	expect(crashed).toBe(true);

	await user.keyboard("r");
	advance(100);
	expect(screen.getByText("Rewinding 5 seconds")).toBeInTheDocument();
	// 2 s more finishes the replay (5 s of play at 2.5x), while the
	// explosion alone would still be running: it lasts 2.5 s and would
	// have been playing for about 2.2 s.
	for (let i = 0; i < 20; i++) advance(100);
	expect(screen.queryByText("Rewinding 5 seconds")).toBeNull();
	expect(advance(16)).toBe(true);
	unmount();
});
