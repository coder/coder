// Track the current animation so a new scroll request cancels
// any in-flight animation before starting.
let activeRafId: number | null = null;

/**
 * Scroll the chat to a specific user-message sentinel, centered
 * in the viewport. Disables browser scroll-anchoring and the
 * sticky-message scroll handler during the animation to prevent
 * layout-shift snap-back.
 */
export function scrollToUserSentinel(messageId: number): void {
	// Cancel any in-flight animation first.
	if (activeRafId !== null) {
		cancelAnimationFrame(activeRafId);
		activeRafId = null;
	}

	const sentinel = document.querySelector(
		`[data-user-sentinel][data-user-message-id="${messageId}"]`,
	);
	if (!sentinel) return;

	const scroller = sentinel.closest(
		'[data-testid="scroll-container"]',
	) as HTMLElement | null;
	if (!scroller) return;

	scroller.style.overflowAnchor = "none";
	scroller.setAttribute("data-scroll-lock", "");

	const offset =
		sentinel.getBoundingClientRect().top -
		scroller.getBoundingClientRect().top -
		scroller.clientHeight / 2;

	// Respect prefers-reduced-motion: jump instantly.
	const prefersReduced = window.matchMedia(
		"(prefers-reduced-motion: reduce)",
	).matches;

	if (prefersReduced) {
		scroller.scrollTop += offset;
		scroller.style.overflowAnchor = "";
		scroller.removeAttribute("data-scroll-lock");
		scroller.dispatchEvent(new Event("scroll"));
		activeRafId = null;
		return;
	}

	const start = scroller.scrollTop;
	const duration = 450;
	const t0 = performance.now();

	// Ease-in-out cubic for a gentle start and gentle stop.
	const ease = (t: number) =>
		t < 0.5 ? 4 * t ** 3 : 1 - (-2 * t + 2) ** 3 / 2;

	const step = (now: number) => {
		const p = Math.min((now - t0) / duration, 1);
		scroller.scrollTop = start + offset * ease(p);
		if (p < 1) {
			activeRafId = requestAnimationFrame(step);
		} else {
			scroller.style.overflowAnchor = "";
			scroller.removeAttribute("data-scroll-lock");
			// Kick all sticky handlers so they recalculate
			// visibility after the jump.
			scroller.dispatchEvent(new Event("scroll"));
			activeRafId = null;
		}
	};
	activeRafId = requestAnimationFrame(step);
}
