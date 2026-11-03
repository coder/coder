/**
 * Drives the Halloween "Ghost in the shell" logo on the SVG nodes directly, so
 * React doesn't re-render on every glitch frame. The state is mirrored on the
 * <svg> as `data-state` (light | glitching | dark) and `data-eyes` (open | closed).
 */

/** The GhostLogo SVG nodes the controller animates. */
export type GhostLogoParts = {
	svg: SVGSVGElement;
	/** Wraps every layer: the glitch filter and the vertical jitter go here. */
	screen: SVGGElement;
	turbulence: SVGFETurbulenceElement;
	displacement: SVGFEDisplacementMapElement;
	/** The dark layer: backlight, silhouette and eyes. */
	ghost: SVGGElement;
	backlight: SVGGElement;
	eyes: SVGGElement;
	/** Scanlines and the rolling bar, shown only mid-glitch. */
	scanlines: SVGGElement;
	bar: SVGRectElement;
	glitchFilterId: string;
};

/** Starts the automatic cycle, feeds it hover changes, and stops it on unmount. */
export type GhostLogoController = {
	start: () => void;
	hover: (hovering: boolean) => void;
	destroy: () => void;
};

/** Every delay in the effect, in milliseconds, plus the glitch frame count. */
export const GHOST_LOGO_TIMING = {
	firstCycleMs: 2000,
	cycleMs: 9000,
	frameMs: 45,
	frames: 10,
	eyesOpenAfterMs: 260,
	blinkAfterMs: 380,
	blinkMs: 500,
	holdMs: 500,
	glitchOutAfterMs: 160,
	hoverEyesAfterMs: 200,
	hoverGlitchOutAfterMs: 140,
	hoverBlinkMinMs: 1600,
	hoverBlinkJitterMs: 1400,
} as const;

const timing = GHOST_LOGO_TIMING;

const BLINK: Keyframe[] = [
	{ transform: "scaleY(1)" },
	{ transform: "scaleY(0.1)", offset: 0.3 },
	{ transform: "scaleY(1)", offset: 0.45 },
	{ transform: "scaleY(0.1)", offset: 0.7 },
	{ transform: "scaleY(1)", offset: 0.85 },
	{ transform: "scaleY(1)" },
];

const BLOOM: Keyframe[] = [
	{ opacity: 0, transform: "scale(0.94)" },
	{ opacity: 0.9, transform: "scale(1.02)", offset: 0.7 },
	{ opacity: 0.9, transform: "scale(1)" },
];

// The effect is decorative, and jsdom has no Web Animations, so skip quietly.
const play = (
	el: Element,
	keyframes: Keyframe[],
	options: number | KeyframeAnimationOptions,
) => {
	if (typeof el.animate === "function") {
		el.animate(keyframes, options);
	}
};

/**
 * Creates a controller for one mounted GhostLogo. `random` is injectable so
 * tests can make the glitch deterministic.
 */
export const createGhostLogoController = (
	parts: GhostLogoParts,
	random: () => number = Math.random,
): GhostLogoController => {
	const abort = new AbortController();
	const { signal } = abort;
	let cycleTimer: ReturnType<typeof setTimeout> | undefined;
	let busy = false;
	let dark = false;
	let hovering = false;
	let hoverRun = 0;

	const sleep = (ms: number) =>
		new Promise<void>((resolve, reject) => {
			if (signal.aborted) {
				reject(signal.reason);
				return;
			}
			const onAbort = () => {
				clearTimeout(timer);
				reject(signal.reason);
			};
			// Remove the listener once the timer fires, or every sleep would leave one
			// on the signal for the life of the page.
			const timer = setTimeout(() => {
				signal.removeEventListener("abort", onAbort);
				resolve();
			}, ms);
			signal.addEventListener("abort", onAbort, { once: true });
		});

	// After destroy, pending sleeps reject with the abort reason; that's expected.
	const run = (task: () => Promise<void>) => {
		task().catch((error) => {
			if (!signal.aborted) {
				throw error;
			}
		});
	};

	const showGhost = (on: boolean, midGlitch: boolean) => {
		parts.ghost.style.opacity = on ? "1" : "0";
		if (!on) {
			parts.backlight.style.opacity = "0";
			return;
		}
		// Mid-glitch dark frames keep some backlight so the silhouette never vanishes.
		parts.backlight.style.opacity = midGlitch ? "0.55" : "0.9";
	};

	const setEyes = (open: boolean) => {
		parts.svg.dataset.eyes = open ? "open" : "closed";
		parts.eyes.style.transform = open ? "scaleY(1)" : "scaleY(0)";
	};

	const blink = async () => {
		play(parts.eyes, BLINK, {
			duration: timing.blinkMs,
			easing: "ease-in-out",
		});
		await sleep(timing.blinkMs + 20);
	};

	/**
	 * Each frame picks light or dark at random, weighted toward the target, so
	 * the logo stutters between the two before it lands.
	 */
	const glitch = async (toDark: boolean) => {
		parts.svg.dataset.state = "glitching";
		parts.screen.setAttribute("filter", `url(#${parts.glitchFilterId})`);
		parts.scanlines.style.opacity = "1";
		for (let i = 0; i < timing.frames; i++) {
			const progress = i / (timing.frames - 1);
			const tear =
				Math.sin(Math.PI * Math.min(1, progress * 1.15)) * (22 + random() * 18);
			parts.turbulence.setAttribute(
				"seed",
				String(1 + Math.floor(random() * 999)),
			);
			parts.turbulence.setAttribute(
				"baseFrequency",
				`0.0001 ${(0.06 + random() * 0.14).toFixed(3)}`,
			);
			parts.displacement.setAttribute("scale", tear.toFixed(1));
			parts.screen.style.transform =
				random() < 0.35
					? `translateY(${((random() - 0.5) * 4).toFixed(1)}px)`
					: "";
			parts.bar.setAttribute("y", String(-6 + ((i * 9 + random() * 6) % 70)));
			const darkChance = toDark ? 0.25 + progress * 0.7 : 0.75 - progress * 0.7;
			showGhost(random() < darkChance, true);
			await sleep(timing.frameMs);
		}
		parts.screen.removeAttribute("filter");
		parts.screen.style.transform = "";
		parts.scanlines.style.opacity = "0";
		dark = toDark;
		showGhost(toDark, false);
		parts.svg.dataset.state = toDark ? "dark" : "light";
		if (toDark) {
			play(parts.backlight, BLOOM, { duration: 550, easing: "ease-out" });
		}
	};

	const cycle = async () => {
		if (busy || hovering) {
			return;
		}
		busy = true;
		try {
			await glitch(true);
			await sleep(timing.eyesOpenAfterMs);
			setEyes(true);
			await sleep(timing.blinkAfterMs);
			await blink();
			await sleep(timing.holdMs);
			if (!hovering) {
				setEyes(false);
				await sleep(timing.glitchOutAfterMs);
				await glitch(false);
			}
		} finally {
			busy = false;
		}
	};

	const waitUntilIdle = async () => {
		while (busy) {
			await sleep(30);
		}
	};

	const hover = async (on: boolean) => {
		hovering = on;
		const thisRun = ++hoverRun;
		await waitUntilIdle();
		if (thisRun !== hoverRun) {
			return; // a newer enter or leave took over
		}
		busy = true;
		try {
			if (on) {
				if (!dark) {
					await glitch(true);
				}
				await sleep(timing.hoverEyesAfterMs);
				// A leave during the glitch-in owns the eyes now; don't flash them open.
				if (thisRun === hoverRun) {
					setEyes(true);
				}
			} else {
				setEyes(false);
				await sleep(timing.hoverGlitchOutAfterMs);
				if (dark) {
					await glitch(false);
				}
			}
		} finally {
			busy = false;
		}
		while (on && hovering && thisRun === hoverRun) {
			await sleep(
				timing.hoverBlinkMinMs + random() * timing.hoverBlinkJitterMs,
			);
			if (hovering && thisRun === hoverRun && !busy) {
				await blink();
			}
		}
	};

	return {
		start: () => {
			const tick = () => {
				run(cycle);
				cycleTimer = setTimeout(tick, timing.cycleMs);
			};
			cycleTimer = setTimeout(tick, timing.firstCycleMs);
		},
		hover: (on) => run(() => hover(on)),
		destroy: () => {
			clearTimeout(cycleTimer);
			abort.abort();
		},
	};
};
