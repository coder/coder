import { describe, expect, it } from "vitest";
import { SmoothTextEngine, STREAM_SMOOTHING } from "./SmoothText";

function makeText(length: number): string {
	return "x".repeat(length);
}

describe("SmoothTextEngine", () => {
	it("reveals text steadily and reaches full length", () => {
		const engine = new SmoothTextEngine();
		const fullText = makeText(200);

		engine.update(fullText, true, false);

		let previousLength = engine.visibleLength;
		let reachedFullLength = false;

		for (let i = 0; i < 600; i++) {
			const nextLength = engine.tick(16);
			expect(nextLength).toBeGreaterThanOrEqual(previousLength);
			previousLength = nextLength;

			if (nextLength === fullText.length) {
				reachedFullLength = true;
				break;
			}
		}

		expect(reachedFullLength).toBe(true);
		expect(engine.visibleLength).toBe(fullText.length);
		expect(engine.isCaughtUp).toBe(true);
	});

	it("accelerates reveal speed when backlog is large", () => {
		const engine = new SmoothTextEngine();
		const fullText = makeText(500);

		engine.update(fullText, true, false);

		let previousLength = engine.visibleLength;
		let revealedCharsInFirst20Ticks = 0;

		for (let i = 0; i < 20; i++) {
			const nextLength = engine.tick(16);
			revealedCharsInFirst20Ticks += nextLength - previousLength;
			previousLength = nextLength;
		}

		// Baseline low-backlog behavior reveals ~1 char/frame with MIN_FRAME_CHARS.
		// A large backlog should reveal multiple chars/frame on average.
		expect(revealedCharsInFirst20Ticks).toBeGreaterThan(20);
	});

	it("caps visual lag when incoming text jumps ahead", () => {
		const engine = new SmoothTextEngine();

		engine.update(makeText(40), true, false);

		while (!engine.isCaughtUp) {
			engine.tick(16);
		}

		engine.update(makeText(420), true, false);

		expect(420 - engine.visibleLength).toBeLessThanOrEqual(
			STREAM_SMOOTHING.MAX_VISUAL_LAG_CHARS,
		);
	});

	it("flushes immediately when streaming ends", () => {
		const engine = new SmoothTextEngine();
		const fullText = makeText(120);

		engine.update(fullText, true, false);

		for (let i = 0; i < 15; i++) {
			engine.tick(16);
		}

		expect(engine.visibleLength).toBeLessThan(fullText.length);

		engine.update(fullText, false, false);

		expect(engine.visibleLength).toBe(fullText.length);
		expect(engine.isCaughtUp).toBe(true);
	});

	it("bypasses smoothing and returns full length immediately", () => {
		const engine = new SmoothTextEngine();
		const fullText = makeText(80);

		engine.update(fullText, true, true);

		expect(engine.visibleLength).toBe(fullText.length);
		expect(engine.isCaughtUp).toBe(true);
	});

	it("clamps visible length when content shrinks", () => {
		const engine = new SmoothTextEngine();

		engine.update(makeText(100), true, false);

		while (engine.visibleLength < 50) {
			engine.tick(16);
		}

		engine.update(makeText(30), true, false);

		expect(engine.visibleLength).toBe(30);
	});

	it("does not force reveal when budget is below one char", () => {
		const engine = new SmoothTextEngine();
		// With a 1-char backlog, adaptive rate is at floor (~24 cps).
		// At 4ms per tick: 24 * 0.004 = 0.096 budget per tick.
		// Budget reaches 1.0 after ceil(1 / 0.096) ≈ 11 ticks.
		engine.update("x", true, false);

		// First tick at 4ms should not reveal (budget ~0.10).
		const afterFirstTick = engine.tick(4);
		expect(afterFirstTick).toBe(0);

		// Several more small ticks should still not reveal.
		engine.tick(4);
		engine.tick(4);
		expect(engine.visibleLength).toBe(0);

		// After enough ticks to accumulate >= 1 char, it should reveal.
		for (let i = 0; i < 20; i++) {
			engine.tick(4);
		}
		expect(engine.visibleLength).toBeGreaterThan(0);
	});

	it("keeps reveal near frame-rate invariant over equal wall time", () => {
		const run = (frameMs: number) => {
			const engine = new SmoothTextEngine();
			engine.update(makeText(400), true, false);
			for (let t = 0; t < 1000; t += frameMs) {
				engine.tick(frameMs);
			}
			return engine.visibleLength;
		};

		const at60Hz = run(16);
		const at240Hz = run(4);

		// Over 1 second of wall time, both refresh rates should reveal
		// approximately the same number of characters.
		expect(Math.abs(at60Hz - at240Hz)).toBeLessThanOrEqual(2);
	});
});
