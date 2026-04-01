import { useEffect, useRef } from "react";
import { useEffectEvent } from "#/hooks/hookPolyfills";

// Minimum visible height in pixels before the clip bottoms out.
const MIN_HEIGHT = 72;

// Vertical padding subtracted when computing the visible portion
// of a clipped container. Keeps the bottom edge of the sticky
// overlay from sitting flush against the scroller top.
const TOP_PADDING = 48;

// Fraction of scroller height above which a sticky message
// is too tall for the clipping overlay.
const TOO_TALL_RATIO = 0.75;

// Minimum clipped pixels before showing the fade gradient.
const FADE_THRESHOLD = 8;

/**
 * A registered sticky-message entry. Each StickyUserMessage
 * provides its sentinel, container, and a state callback.
 */
export interface StickyEntry {
	sentinel: HTMLElement;
	container: HTMLElement;
	onTooTallChange: (isTooTall: boolean) => void;
}

/**
 * Shared scroll handler that replaces per-instance scroll
 * listeners, ResizeObservers, and window resize listeners for
 * StickyUserMessage instances.
 *
 * The key design principle is batching all DOM reads before all
 * DOM writes in each animation frame to eliminate layout thrashing.
 *
 * Returns stable `register` / `unregister` callbacks. Call
 * `register` when a StickyUserMessage mounts and `unregister`
 * when it unmounts.
 */
export function useStickyScrollHandler(scroller: HTMLElement | null): {
	register: (entry: StickyEntry) => void;
	unregister: (entry: StickyEntry) => void;
} {
	const entriesRef = useRef(new Set<StickyEntry>());
	const rafIdRef = useRef<number | null>(null);

	// Core update that batches all DOM reads before writes.
	// Wrapped in useEffectEvent so it always sees the current
	// scroller without appearing in any dependency array.
	const runUpdate = useEffectEvent(() => {
		if (!scroller) return;
		const entries = entriesRef.current;
		if (entries.size === 0) return;

		// ---- Read phase ----
		// Touch every layout-triggering property up-front so
		// the browser only recalculates layout once.
		const scrollerTop = scroller.getBoundingClientRect().top;
		const scrollerHeight = scroller.clientHeight;

		const measurements: Array<{
			entry: StickyEntry;
			fullHeight: number;
			sentinelTop: number;
		}> = [];

		for (const entry of entries) {
			measurements.push({
				entry,
				fullHeight: entry.container.offsetHeight,
				sentinelTop: entry.sentinel.getBoundingClientRect().top,
			});
		}

		// ---- Write phase ----
		// Mutate styles and fire callbacks only after every
		// entry has been measured.
		for (const { entry, fullHeight, sentinelTop } of measurements) {
			const isTooTall = fullHeight > scrollerHeight * TOO_TALL_RATIO;
			entry.onTooTallChange(isTooTall);

			if (isTooTall) {
				entry.container.style.setProperty("--clip-h", `${fullHeight}px`);
				entry.container.style.setProperty("--fade-opacity", "0");
				continue;
			}

			const scrolledPast = scrollerTop - sentinelTop;

			if (scrolledPast <= 0) {
				// Sentinel is still in view. Set the full
				// height so --clip-h is correct the instant
				// the sticky overlay appears.
				entry.container.style.setProperty("--clip-h", `${fullHeight}px`);
				entry.container.style.setProperty("--fade-opacity", "0");
				continue;
			}

			const visible = Math.max(
				fullHeight - scrolledPast - TOP_PADDING,
				MIN_HEIGHT,
			);
			entry.container.style.setProperty("--clip-h", `${visible}px`);
			// Show the fade gradient only once enough content
			// is clipped to be visually meaningful.
			entry.container.style.setProperty(
				"--fade-opacity",
				visible < fullHeight - FADE_THRESHOLD ? "1" : "0",
			);
		}
	});

	// Coalesce calls into one update per animation frame.
	const scheduleUpdate = useEffectEvent(() => {
		if (rafIdRef.current !== null) return;
		rafIdRef.current = requestAnimationFrame(() => {
			rafIdRef.current = null;
			runUpdate();
		});
	});

	// Attach a single scroll listener and ResizeObserver when
	// the scroller element is available.
	useEffect(() => {
		if (!scroller) return;

		scroller.addEventListener("scroll", scheduleUpdate, {
			passive: true,
		});

		// The ResizeObserver replaces per-instance window resize
		// listeners (scrollerHeight changes) and content resize
		// observers (streaming responses growing the transcript
		// in flex-col-reverse where no scroll event fires).
		const ro = new ResizeObserver(() => scheduleUpdate());
		ro.observe(scroller);

		// Also observe the content wrapper so we catch height
		// changes from streaming responses in flex-col-reverse
		// layouts where scrollTop stays pinned at zero.
		const contentEl = scroller.firstElementChild;
		if (contentEl) {
			ro.observe(contentEl);
		}

		return () => {
			scroller.removeEventListener("scroll", scheduleUpdate);
			ro.disconnect();
			if (rafIdRef.current !== null) {
				cancelAnimationFrame(rafIdRef.current);
				rafIdRef.current = null;
			}
		};
	}, [scroller, scheduleUpdate]);

	const register = useEffectEvent((entry: StickyEntry) => {
		entriesRef.current.add(entry);
		// Run an initial update so the entry gets correct
		// values before the next paint.
		scheduleUpdate();
	});

	const unregister = useEffectEvent((entry: StickyEntry) => {
		entriesRef.current.delete(entry);
	});

	return { register, unregister };
}
