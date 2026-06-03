let activeRafId: number | null = null;
let activeScroller: HTMLElement | null = null;

const SCROLL_DURATION_MS = 450;

// Wait for Radix popover exit animation to complete before
// scrolling. Without this, the popover's unmount triggers a
// layout shift that fights the scroll animation.
const POPOVER_CLOSE_DELAY_MS = 80;

function unlock() {
	if (activeScroller) {
		activeScroller.style.overflowAnchor = "";
		activeScroller.removeAttribute("data-scroll-lock");
		activeScroller.dispatchEvent(new Event("scroll"));
		activeScroller = null;
	}
}

/** Scroll to a user-message sentinel. Disables scroll-anchoring during animation to prevent snap-back. */
export function scrollToUserSentinel(messageId: number): boolean {
	if (activeRafId !== null) {
		cancelAnimationFrame(activeRafId);
		activeRafId = null;
		unlock();
	}

	const sentinel = document.querySelector(
		`[data-user-sentinel][data-user-message-id="${messageId}"]`,
	);
	if (!sentinel) return false;

	const scroller = sentinel.closest(".overflow-y-auto") as HTMLElement | null;
	if (!scroller) return false;

	activeScroller = scroller;
	scroller.style.overflowAnchor = "none";
	scroller.setAttribute("data-scroll-lock", "");

	const offset =
		sentinel.getBoundingClientRect().top -
		scroller.getBoundingClientRect().top -
		scroller.clientHeight / 2;

	const prefersReduced = window.matchMedia(
		"(prefers-reduced-motion: reduce)",
	).matches;

	if (prefersReduced) {
		scroller.scrollTop += offset;
		unlock();
		activeRafId = null;
		return true;
	}

	const start = scroller.scrollTop;
	const t0 = performance.now();

	const ease = (t: number) =>
		t < 0.5 ? 4 * t ** 3 : 1 - (-2 * t + 2) ** 3 / 2;

	const step = (now: number) => {
		const p = Math.min((now - t0) / SCROLL_DURATION_MS, 1);
		scroller.scrollTop = start + offset * ease(p);
		if (p < 1) {
			activeRafId = requestAnimationFrame(step);
		} else {
			unlock();
			activeRafId = null;
		}
	};
	activeRafId = requestAnimationFrame(step);
	return true;
}

export function scrollToUserSentinelAfterClose(messageId: number): void {
	setTimeout(() => scrollToUserSentinel(messageId), POPOVER_CLOSE_DELAY_MS);
}

/** @internal */
export function _resetForTesting(): void {
	if (activeRafId !== null) {
		cancelAnimationFrame(activeRafId);
		activeRafId = null;
	}
	activeScroller = null;
}
