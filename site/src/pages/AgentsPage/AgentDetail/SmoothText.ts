import { useEffect, useRef, useState } from "react";

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
 * manages only the presentation clock (visible prefix length) using
 * a character budget model.
 */
export class SmoothTextEngine {
	private fullLength = 0;
	private visibleLengthValue = 0;
	private charBudget = 0;
	private isStreaming = false;
	private bypassSmoothing = false;

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

	/**
	 * Update the ingested text and stream state.
	 */
	update(
		fullText: string,
		isStreaming: boolean,
		bypassSmoothing: boolean,
	): void {
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
			return;
		}

		this.enforceMaxVisualLag();
	}

	/**
	 * Advance the presentation clock by a timestep.
	 */
	tick(dtMs: number): number {
		if (dtMs <= 0) {
			return this.visibleLengthValue;
		}

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
	 * Reset all engine state, typically when a new stream starts.
	 */
	reset(): void {
		this.fullLength = 0;
		this.visibleLengthValue = 0;
		this.charBudget = 0;
		this.isStreaming = false;
		this.bypassSmoothing = false;
	}
}

// ── Hook ────────────────────────────────────────────────────────────

export interface UseSmoothStreamingTextOptions {
	fullText: string;
	isStreaming: boolean;
	bypassSmoothing: boolean;
	/** Changing this resets the engine (new stream). */
	streamKey: string;
}

export interface UseSmoothStreamingTextResult {
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
	const engineRef = useRef(new SmoothTextEngine());
	const previousStreamKeyRef = useRef(options.streamKey);

	if (previousStreamKeyRef.current !== options.streamKey) {
		engineRef.current.reset();
		previousStreamKeyRef.current = options.streamKey;
	}

	const engine = engineRef.current;
	engine.update(options.fullText, options.isStreaming, options.bypassSmoothing);

	const [visibleLength, setVisibleLength] = useState(
		() => engine.visibleLength,
	);
	const visibleLengthRef = useRef(visibleLength);
	visibleLengthRef.current = visibleLength;

	const rafIdRef = useRef<number | null>(null);
	const previousTimestampRef = useRef<number | null>(null);

	// Frame callback stored as a ref so effects don't depend on it,
	// preventing teardown/restart of the RAF loop on every text
	// delta. Reads from refs and the stable engine instance, so the
	// captured closure is always correct.
	const frameRef = useRef<FrameRequestCallback>(null!);
	frameRef.current = (timestampMs: number) => {
		if (previousTimestampRef.current !== null) {
			const nextLength = engine.tick(
				timestampMs - previousTimestampRef.current,
			);
			if (nextLength !== visibleLengthRef.current) {
				visibleLengthRef.current = nextLength;
				setVisibleLength(nextLength);
			}
		}
		previousTimestampRef.current = timestampMs;
		if (!engine.isCaughtUp) {
			rafIdRef.current = requestAnimationFrame(frameRef.current);
		} else {
			rafIdRef.current = null;
			previousTimestampRef.current = null;
		}
	};

	// Sync engine state → React and re-arm RAF when new deltas
	// arrive after catch-up. No cleanup: this effect only observes +
	// one-shot starts; the lifecycle effect below owns resource
	// teardown.
	// biome-ignore lint/correctness/useExhaustiveDependencies: fullText and streamKey are intentional triggers to re-arm the RAF loop when new text arrives or the stream resets
	useEffect(() => {
		if (visibleLengthRef.current !== engine.visibleLength) {
			visibleLengthRef.current = engine.visibleLength;
			setVisibleLength(engine.visibleLength);
		}

		if (
			rafIdRef.current === null &&
			options.isStreaming &&
			!options.bypassSmoothing &&
			!engine.isCaughtUp
		) {
			rafIdRef.current = requestAnimationFrame(frameRef.current);
		}
	}, [
		engine,
		options.fullText,
		options.isStreaming,
		options.bypassSmoothing,
		options.streamKey,
	]);

	// Lifecycle: stop RAF when streaming ends or stream key changes,
	// and on unmount.
	// biome-ignore lint/correctness/useExhaustiveDependencies: streamKey is an intentional trigger to reset RAF state on new streams
	useEffect(() => {
		if (!options.isStreaming || options.bypassSmoothing) {
			if (rafIdRef.current !== null) {
				cancelAnimationFrame(rafIdRef.current);
			}
			rafIdRef.current = null;
			previousTimestampRef.current = null;
		}
		return () => {
			if (rafIdRef.current !== null) {
				cancelAnimationFrame(rafIdRef.current);
			}
			rafIdRef.current = null;
			previousTimestampRef.current = null;
		};
	}, [options.isStreaming, options.bypassSmoothing, options.streamKey]);

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
