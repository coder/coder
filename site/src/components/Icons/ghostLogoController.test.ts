import {
	createGhostLogoController,
	type GhostLogoParts,
	GHOST_LOGO_TIMING as timing,
} from "./ghostLogoController";

const SVG = "http://www.w3.org/2000/svg";

const makeParts = (): GhostLogoParts => {
	const el = <K extends keyof SVGElementTagNameMap>(tag: K) =>
		document.createElementNS(SVG, tag) as SVGElementTagNameMap[K];
	const svg = el("svg");
	svg.dataset.state = "light";
	svg.dataset.eyes = "closed";
	return {
		svg,
		screen: el("g"),
		turbulence: el("feTurbulence"),
		displacement: el("feDisplacementMap"),
		ghost: el("g"),
		backlight: el("g"),
		eyes: el("g"),
		scanlines: el("g"),
		bar: el("rect"),
		glitchFilterId: "glitch",
	};
};

const GLITCH_MS = timing.frames * timing.frameMs;

describe("createGhostLogoController", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it("glitches dark, opens and closes its eyes, and glitches back", async () => {
		const parts = makeParts();
		const controller = createGhostLogoController(parts, () => 0.5);
		controller.start();

		await vi.advanceTimersByTimeAsync(timing.firstCycleMs - 1);
		expect(parts.svg.dataset.state).toBe("light");

		await vi.advanceTimersByTimeAsync(1);
		expect(parts.svg.dataset.state).toBe("glitching");
		expect(parts.screen.getAttribute("filter")).toBe("url(#glitch)");

		await vi.advanceTimersByTimeAsync(GLITCH_MS);
		expect(parts.svg.dataset.state).toBe("dark");
		expect(parts.screen.hasAttribute("filter")).toBe(false);
		expect(parts.ghost.style.opacity).toBe("1");

		await vi.advanceTimersByTimeAsync(timing.eyesOpenAfterMs);
		expect(parts.svg.dataset.eyes).toBe("open");

		await vi.advanceTimersByTimeAsync(
			timing.blinkAfterMs + timing.blinkMs + 20 + timing.holdMs,
		);
		expect(parts.svg.dataset.eyes).toBe("closed");

		await vi.advanceTimersByTimeAsync(timing.glitchOutAfterMs + GLITCH_MS);
		expect(parts.svg.dataset.state).toBe("light");
		expect(parts.ghost.style.opacity).toBe("0");

		controller.destroy();
	});

	it("holds the dark state while hovered and lets go on leave", async () => {
		const parts = makeParts();
		const controller = createGhostLogoController(parts, () => 0.5);
		controller.start();

		controller.hover(true);
		await vi.advanceTimersByTimeAsync(GLITCH_MS + timing.hoverEyesAfterMs);
		expect(parts.svg.dataset.state).toBe("dark");
		expect(parts.svg.dataset.eyes).toBe("open");

		// The automatic cycle doesn't interrupt a hover.
		await vi.advanceTimersByTimeAsync(timing.firstCycleMs + timing.cycleMs);
		expect(parts.svg.dataset.state).toBe("dark");

		controller.hover(false);
		await vi.advanceTimersByTimeAsync(
			timing.blinkMs + timing.hoverGlitchOutAfterMs + GLITCH_MS + 100,
		);
		expect(parts.svg.dataset.state).toBe("light");
		expect(parts.svg.dataset.eyes).toBe("closed");

		controller.destroy();
	});

	it("keeps its eyes closed when the pointer leaves during the glitch-in", async () => {
		const parts = makeParts();
		const controller = createGhostLogoController(parts, () => 0.5);
		controller.start();

		controller.hover(true);
		await vi.advanceTimersByTimeAsync(GLITCH_MS / 2);
		controller.hover(false);
		await vi.advanceTimersByTimeAsync(
			GLITCH_MS / 2 + timing.hoverEyesAfterMs + 1,
		);
		expect(parts.svg.dataset.eyes).toBe("closed");

		await vi.advanceTimersByTimeAsync(
			timing.hoverGlitchOutAfterMs + GLITCH_MS + 100,
		);
		expect(parts.svg.dataset.state).toBe("light");

		controller.destroy();
	});

	it("doesn't accumulate abort listeners across cycles", async () => {
		const add = vi.spyOn(AbortSignal.prototype, "addEventListener");
		const remove = vi.spyOn(AbortSignal.prototype, "removeEventListener");
		const parts = makeParts();
		const controller = createGhostLogoController(parts, () => 0.5);
		controller.start();

		await vi.advanceTimersByTimeAsync(timing.firstCycleMs + timing.cycleMs * 3);
		expect(add.mock.calls.length).toBeGreaterThan(30);
		expect(
			add.mock.calls.length - remove.mock.calls.length,
		).toBeLessThanOrEqual(1);

		controller.destroy();
		add.mockRestore();
		remove.mockRestore();
	});

	it("stops everything on destroy", async () => {
		const parts = makeParts();
		const controller = createGhostLogoController(parts, () => 0.5);
		controller.start();
		await vi.advanceTimersByTimeAsync(timing.firstCycleMs + 2 * timing.frameMs);
		expect(parts.svg.dataset.state).toBe("glitching");

		controller.destroy();
		await vi.advanceTimersByTimeAsync(timing.cycleMs * 2);
		expect(parts.svg.dataset.state).toBe("glitching");
		expect(vi.getTimerCount()).toBe(0);
	});
});
