// Coordinates layout reads and writes across all StickyUserMessage
// instances sharing the same scroll container. Without coordination,
// each instance creates its own ResizeObserver on the content wrapper
// and its own scroll listener. When N instances each call
// getBoundingClientRect() (forced layout) then style.top = ...
// (layout invalidation), we get N forced layout recalculations per
// frame. Safari is especially sensitive to this pattern and may
// adjust scrollTop during the layout thrashing, causing the scroll
// position to drift away from the bottom.
//
// The coordinator creates a single ResizeObserver and scroll listener
// per scroller element. On each tick, it runs ALL read callbacks
// (getBoundingClientRect) before ANY write callbacks (style mutations),
// so the browser computes layout exactly once per frame.

interface StickyRegistration {
	/** DOM reads: getBoundingClientRect, offsetHeight, etc. */
	read: (scrollerTop: number, scrollerHeight: number) => void;
	/** DOM writes: style.top, style.setProperty, setState. */
	write: () => void;
}

interface Coordinator {
	refCount: number;
	registrations: Set<StickyRegistration>;
	destroy: () => void;
}

const coordinators = new WeakMap<Element, Coordinator>();

/**
 * Register a StickyUserMessage's read/write callbacks with the
 * shared coordinator for the given scroller element. Returns a
 * cleanup function that unregisters the callbacks and tears down
 * the coordinator when the last instance unmounts.
 */
export function registerStickyUpdate(
	scroller: HTMLElement,
	registration: StickyRegistration,
): () => void {
	let coord = coordinators.get(scroller);
	if (!coord) {
		coord = createCoordinator(scroller);
		coordinators.set(scroller, coord);
	}

	coord.refCount++;
	coord.registrations.add(registration);

	return () => {
		const c = coordinators.get(scroller);
		if (!c) return;
		c.registrations.delete(registration);
		c.refCount--;
		if (c.refCount <= 0) {
			c.destroy();
			coordinators.delete(scroller);
		}
	};
}

function createCoordinator(scroller: HTMLElement): Coordinator {
	const registrations = new Set<StickyRegistration>();
	let scrollRafId: number | null = null;
	let resizeRafId: number | null = null;
	let scrollerTop = scroller.getBoundingClientRect().top;
	let scrollerHeight = scroller.clientHeight;

	const runBatchedUpdate = () => {
		// Read phase: all instances measure their DOM positions.
		// No writes happen between reads, so the browser computes
		// layout exactly once.
		for (const reg of registrations) {
			reg.read(scrollerTop, scrollerHeight);
		}
		// Write phase: all instances apply their style mutations.
		for (const reg of registrations) {
			reg.write();
		}
	};

	const onScroll = () => {
		if (scrollRafId !== null) return;
		scrollRafId = requestAnimationFrame(() => {
			scrollRafId = null;
			runBatchedUpdate();
		});
	};

	const onResize = () => {
		scrollerTop = scroller.getBoundingClientRect().top;
		scrollerHeight = scroller.clientHeight;
		runBatchedUpdate();
	};

	const contentEl = scroller.firstElementChild as HTMLElement | null;
	const contentObserver = new ResizeObserver(() => {
		if (resizeRafId !== null) return;
		resizeRafId = requestAnimationFrame(() => {
			resizeRafId = null;
			runBatchedUpdate();
		});
	});
	if (contentEl) {
		contentObserver.observe(contentEl);
	}

	scroller.addEventListener("scroll", onScroll, { passive: true });
	window.addEventListener("resize", onResize);

	const coord: Coordinator = {
		refCount: 0,
		registrations,
		destroy() {
			scroller.removeEventListener("scroll", onScroll);
			window.removeEventListener("resize", onResize);
			contentObserver.disconnect();
			if (scrollRafId !== null) cancelAnimationFrame(scrollRafId);
			if (resizeRafId !== null) cancelAnimationFrame(resizeRafId);
		},
	};

	return coord;
}
