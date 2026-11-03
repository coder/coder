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
		eyes: el("g"),
		scanlines: el("g"),
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

	it("glitches dark and opens its eyes on hover, and glitches back on leave", async () => {
		const parts = makeParts();
		const controller = createGhostLogoController(parts, () => 0.5);

		controller.hover(true);
		await vi.advanceTimersByTimeAsync(timing.frameMs);
		expect(parts.svg.dataset.state).toBe("glitching");
		expect(parts.screen.getAttribute("filter")).toBe("url(#glitch)");

		await vi.advanceTimersByTimeAsync(GLITCH_MS + timing.eyesOpenAfterMs);
		expect(parts.svg.dataset.state).toBe("dark");
		expect(parts.svg.dataset.eyes).toBe("open");
		expect(parts.screen.hasAttribute("filter")).toBe(false);

		controller.hover(false);
		await vi.advanceTimersByTimeAsync(timing.glitchOutAfterMs + GLITCH_MS);
		expect(parts.svg.dataset.state).toBe("light");
		expect(parts.svg.dataset.eyes).toBe("closed");
		expect(parts.ghost.style.opacity).toBe("0");

		controller.destroy();
	});

	it("keeps its eyes closed when the pointer leaves during the glitch-in", async () => {
		const parts = makeParts();
		const controller = createGhostLogoController(parts, () => 0.5);

		controller.hover(true);
		await vi.advanceTimersByTimeAsync(GLITCH_MS / 2);
		controller.hover(false);
		await vi.advanceTimersByTimeAsync(
			GLITCH_MS / 2 + timing.eyesOpenAfterMs + 1,
		);
		expect(parts.svg.dataset.eyes).toBe("closed");

		await vi.advanceTimersByTimeAsync(timing.glitchOutAfterMs + GLITCH_MS);
		expect(parts.svg.dataset.state).toBe("light");

		controller.destroy();
	});

	it("doesn't accumulate abort listeners across hovers", async () => {
		const add = vi.spyOn(AbortSignal.prototype, "addEventListener");
		const remove = vi.spyOn(AbortSignal.prototype, "removeEventListener");
		const controller = createGhostLogoController(makeParts(), () => 0.5);

		for (let i = 0; i < 3; i++) {
			controller.hover(true);
			await vi.advanceTimersByTimeAsync(2000);
			controller.hover(false);
			await vi.advanceTimersByTimeAsync(2000);
		}
		expect(add.mock.calls.length).toBeGreaterThan(30);
		expect(add.mock.calls.length).toBe(remove.mock.calls.length);

		controller.destroy();
		add.mockRestore();
		remove.mockRestore();
	});

	it("stops everything on destroy", async () => {
		const parts = makeParts();
		const controller = createGhostLogoController(parts, () => 0.5);
		controller.hover(true);
		await vi.advanceTimersByTimeAsync(2 * timing.frameMs);
		expect(parts.svg.dataset.state).toBe("glitching");

		controller.destroy();
		await vi.advanceTimersByTimeAsync(5000);
		expect(parts.svg.dataset.state).toBe("glitching");
		expect(vi.getTimerCount()).toBe(0);
	});
});
