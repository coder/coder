import { useEffect, useState, useSyncExternalStore } from "react";

// Smooth streaming presentation constants. These control the jitter
// buffer that makes streamed text appear at a steady cadence instead
// of bursty token clumps. Internal-only; no user-facing setting.
export const STREAM_SMOOTHING = {
	/** Baseline reveal speed in characters per second. */
	BASE_CHARS_PER_SEC: 72,
	/** Floor — never slower than this even when buffer is nearly empty. */
	MIN_CHARS_PER_SEC: 24,
	/** Ceiling — hard cap to prevent overwhelming the markdown renderer. */
	MAX_CHARS_PER_SEC: 420,
	/** Backlog level where adaptive reveal runs at MAX_CHARS_PER_SEC. */
	CATCHUP_BACKLOG_CHARS: 180,
	/**
	 * Keep the rendered transcript close to live output even during
	 * bursty streams.
	 */
	MAX_VISUAL_LAG_CHARS: 120,
	/** Max characters revealed in a single animation frame. */
	MAX_FRAME_CHARS: 48,
	/**
	 * Min characters revealed per tick once budget permits. Avoids
	 * sub-character stalls.
	 */
	MIN_FRAME_CHARS: 1,
} as const;

function clamp(value: number, min: number, max: number): number {
	return Math.max(min, Math.min(max, value));
}

function getAdaptiveRate(backlog: number): number {
	const backlogPressure = clamp(
		backlog / STREAM_SMOOTHING.CATCHUP_BACKLOG_CHARS,
		0,
		1,
	);

	const targetRate =
		STREAM_SMOOTHING.BASE_CHARS_PER_SEC +
		backlogPressure *
			(STREAM_SMOOTHING.MAX_CHARS_PER_SEC -
				STREAM_SMOOTHING.BASE_CHARS_PER_SEC);

	return clamp(
		targetRate,
		STREAM_SMOOTHING.MIN_CHARS_PER_SEC,
		STREAM_SMOOTHING.MAX_CHARS_PER_SEC,
	);
}

/**
 * Deterministic text reveal engine for smoothing streamed output.
 *
 * The ingestion clock (incoming full text) is external; this class
 * manages the presentation clock (visible prefix length) using a
 * character budget model, and owns the RAF loop that drives it.
 *
 * Implements a subscribe/getSnapshot contract for use with
 * useSyncExternalStore.
 */
export class SmoothTextEngine {
	private fullLength = 0;
	private visibleLengthValue = 0;
	private charBudget = 0;
	private isStreaming = false;
	private bypassSmoothing = false;

	private rafId: number | null = null;
	private previousTimestamp: number | null = null;
	private listeners = new Set<() => void>();

	private enforceMaxVisualLag(): void {
		if (!this.isStreaming || this.bypassSmoothing) {
			return;
		}

		// Keep visible output near the ingested stream so interruption
		// doesn't reveal a large hidden tail all at once.
		const minVisibleLength = Math.max(
			0,
			this.fullLength - STREAM_SMOOTHING.MAX_VISUAL_LAG_CHARS,
		);
		if (this.visibleLengthValue < minVisibleLength) {
			this.visibleLengthValue = minVisibleLength;
			this.charBudget = 0;
		}
	}

	private notify(): void {
		for (const listener of this.listeners) {
			listener();
		}
	}

	private stopLoop(): void {
		if (this.rafId !== null) {
			cancelAnimationFrame(this.rafId);
			this.rafId = null;
		}
		this.previousTimestamp = null;
	}

	private startLoop(): void {
		if (this.rafId !== null) {
			return;
		}
		this.rafId = requestAnimationFrame(this.frame);
	}

	private frame = (timestampMs: number): void => {
		if (this.previousTimestamp !== null) {
			const dtMs = timestampMs - this.previousTimestamp;
			const prevLength = this.visibleLengthValue;
			this.tick(dtMs);
			if (this.visibleLengthValue !== prevLength) {
				this.notify();
			}
		}
		this.previousTimestamp = timestampMs;

		if (!this.isCaughtUp) {
			this.rafId = requestAnimationFrame(this.frame);
		} else {
			this.rafId = null;
			this.previousTimestamp = null;
		}
	};

	/**
	 * Update the ingested text and stream state. Starts or stops the
	 * internal RAF loop as needed, and notifies subscribers when the
	 * visible length changes synchronously (e.g. bypass, stream end,
	 * content shrink).
	 */
	update(
		fullText: string,
		isStreaming: boolean,
		bypassSmoothing: boolean,
	): void {
		const prevVisible = this.visibleLengthValue;

		this.fullLength = fullText.length;
		this.isStreaming = isStreaming;
		this.bypassSmoothing = bypassSmoothing;

		if (this.fullLength < this.visibleLengthValue) {
			this.visibleLengthValue = this.fullLength;
			this.charBudget = 0;
		}

		if (!isStreaming || bypassSmoothing) {
			this.visibleLengthValue = this.fullLength;
			this.charBudget = 0;
			this.stopLoop();
		} else {
			this.enforceMaxVisualLag();
			if (!this.isCaughtUp) {
				this.startLoop();
			}
		}

		if (this.visibleLengthValue !== prevVisible) {
			this.notify();
		}
	}

	/**
	 * Advance the presentation clock by a timestep.
	 */
	tick(dtMs: number): number {
		if (dtMs <= 0 || Number.isNaN(dtMs)) {
			return this.visibleLengthValue;
		}

		// Clamp to prevent charBudget inflation after long
		// background periods where RAF was paused by the browser.
		dtMs = Math.min(100, dtMs);

		if (!this.isStreaming || this.bypassSmoothing) {
			return this.visibleLengthValue;
		}

		if (this.visibleLengthValue > this.fullLength) {
			this.visibleLengthValue = this.fullLength;
			this.charBudget = 0;
		}

		if (this.visibleLengthValue === this.fullLength) {
			return this.visibleLengthValue;
		}

		const backlog = this.fullLength - this.visibleLengthValue;
		const adaptiveRate = getAdaptiveRate(backlog);

		this.charBudget += adaptiveRate * (dtMs / 1000);

		// Budget-gated reveal: only reveal when at least one whole
		// character has accrued. This makes cadence frame-rate
		// invariant — a 240Hz display accumulates budget across
		// several frames before revealing, rather than forcing
		// 1 char/frame at any refresh rate.
		const wholeCharsReady = Math.floor(this.charBudget);
		if (wholeCharsReady < STREAM_SMOOTHING.MIN_FRAME_CHARS) {
			return this.visibleLengthValue;
		}

		const reveal = Math.min(wholeCharsReady, STREAM_SMOOTHING.MAX_FRAME_CHARS);
		this.visibleLengthValue = Math.min(
			this.fullLength,
			this.visibleLengthValue + reveal,
		);
		this.charBudget -= reveal;

		return this.visibleLengthValue;
	}

	get visibleLength(): number {
		return this.visibleLengthValue;
	}

	get isCaughtUp(): boolean {
		return this.visibleLengthValue === this.fullLength;
	}

	/**
	 * Subscribe to visible length changes. Returns an unsubscribe
	 * function, matching the contract useSyncExternalStore expects.
	 */
	subscribe = (listener: () => void): (() => void) => {
		this.listeners.add(listener);
		return () => {
			this.listeners.delete(listener);
		};
	};

	/**
	 * Reset all engine state, typically when a new stream starts.
	 */
	reset(): void {
		this.stopLoop();
		this.fullLength = 0;
		this.visibleLengthValue = 0;
		this.charBudget = 0;
		this.isStreaming = false;
		this.bypassSmoothing = false;
	}

	/**
	 * Stop the animation loop and release resources. Call when the
	 * engine instance is being discarded.
	 */
	dispose(): void {
		this.stopLoop();
		this.listeners.clear();
	}
}

// ── Hook ────────────────────────────────────────────────────────────

interface UseSmoothStreamingTextOptions {
	fullText: string;
	isStreaming: boolean;
	bypassSmoothing: boolean;
	/** Changing this resets the engine (new stream). */
	streamKey: string;
}

interface UseSmoothStreamingTextResult {
	visibleText: string;
	isCaughtUp: boolean;
}

// Module-scoped grapheme segmenter, created once and shared across
// all hook instances. Falls back to codepoint iteration when the
// Intl.Segmenter API is unavailable.

// Minimal type for the Intl.Segmenter API which is widely supported
// at runtime but not included in all TypeScript lib bundles.
interface GraphemeSegment {
	index: number;
	segment: string;
}

interface GraphemeSegmenterInstance {
	segment(input: string): Iterable<GraphemeSegment>;
}

const graphemeSegmenter: GraphemeSegmenterInstance | null = (() => {
	try {
		const Seg = (Intl as Record<string, unknown>).Segmenter;
		if (typeof Seg === "function") {
			return new (
				Seg as new (
					locales?: string,
					options?: { granularity?: string },
				) => GraphemeSegmenterInstance
			)(undefined, {
				granularity: "grapheme",
			});
		}
	} catch {
		// Fallback to null when Intl.Segmenter is unavailable.
	}
	return null;
})();

/**
 * Slice a string at the largest grapheme-cluster boundary that does
 * not exceed {@link maxCodeUnitLength} UTF-16 code units. When the
 * `Intl.Segmenter` API is available it is used for correct grapheme
 * handling; otherwise the function falls back to iterating by
 * codepoint which still avoids splitting surrogate pairs.
 */
function sliceAtGraphemeBoundary(
	text: string,
	maxCodeUnitLength: number,
): string {
	if (maxCodeUnitLength <= 0) {
		return "";
	}

	if (maxCodeUnitLength >= text.length) {
		return text;
	}

	if (graphemeSegmenter) {
		let safeEnd = 0;
		const segments = Array.from(graphemeSegmenter.segment(text));

		for (const segment of segments) {
			const segmentEnd = segment.index + segment.segment.length;
			if (segmentEnd > maxCodeUnitLength) {
				break;
			}
			safeEnd = segmentEnd;
		}

		return text.slice(0, safeEnd);
	}

	// Fallback: iterate by codepoint to avoid splitting surrogate
	// pairs. This is less precise than grapheme segmentation but
	// still safe for rendering.
	let safeEnd = 0;
	for (const codePoint of Array.from(text)) {
		const codePointEnd = safeEnd + codePoint.length;
		if (codePointEnd > maxCodeUnitLength) {
			break;
		}
		safeEnd = codePointEnd;
	}

	return text.slice(0, safeEnd);
}

export function useSmoothStreamingText(
	options: UseSmoothStreamingTextOptions,
): UseSmoothStreamingTextResult {
	// Store the engine and the streamKey it was created for together
	// in a single useState. When the streamKey changes during render,
	// we dispose the old engine and create a fresh one inline — this
	// is the "derive state from props" pattern React documents for
	// useState, avoiding useEffect for reset logic.
	const [{ engine, streamKey }, setEngineState] = useState(() => ({
		engine: new SmoothTextEngine(),
		streamKey: options.streamKey,
	}));

	if (streamKey !== options.streamKey) {
		engine.dispose();
		const next = new SmoothTextEngine();
		setEngineState({ engine: next, streamKey: options.streamKey });
		// Use the new engine for the rest of this render.
		next.update(options.fullText, options.isStreaming, options.bypassSmoothing);
	} else {
		engine.update(
			options.fullText,
			options.isStreaming,
			options.bypassSmoothing,
		);
	}

	// Dispose on unmount.
	useEffect(() => {
		return () => engine.dispose();
	}, [engine]);

	const visibleLength = useSyncExternalStore(
		engine.subscribe,
		() => engine.visibleLength,
	);

	if (!options.isStreaming || options.bypassSmoothing) {
		return {
			visibleText: options.fullText,
			isCaughtUp: true,
		};
	}

	const visiblePrefixLength = Math.min(
		visibleLength,
		engine.visibleLength,
		options.fullText.length,
	);

	const visibleText = sliceAtGraphemeBoundary(
		options.fullText,
		visiblePrefixLength,
	);

	return {
		visibleText,
		isCaughtUp: visibleText.length === options.fullText.length,
	};
}
