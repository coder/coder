/**
 * Drives the GhostLogo easter egg on the SVG nodes directly, so React doesn't
 * re-render on every glitch frame. The state is mirrored on the <svg> as
 * `data-state` (light | glitching | dark) and `data-eyes` (open | closed).
 */

/** The GhostLogo SVG nodes the controller animates. */
export type GhostLogoParts = {
	svg: SVGSVGElement;
	/** Wraps every layer; the glitch filter goes here. */
	screen: SVGGElement;
	turbulence: SVGFETurbulenceElement;
	displacement: SVGFEDisplacementMapElement;
	/** The dark layer: backlight, silhouette and eyes. */
	ghost: SVGGElement;
	eyes: SVGGElement;
	/** Scanlines, shown only mid-glitch. */
	scanlines: SVGGElement;
	glitchFilterId: string;
};

/** Feeds hover changes to the effect and stops it on unmount. */
export type GhostLogoController = {
	hover: (hovering: boolean) => void;
	destroy: () => void;
};

/** Delays in milliseconds, plus the glitch frame count. */
export const GHOST_LOGO_TIMING = {
	frameMs: 45,
	frames: 10,
	eyesOpenAfterMs: 200,
	glitchOutAfterMs: 140,
	blinkEveryMs: 3000,
} as const;

const timing = GHOST_LOGO_TIMING;

// A double blink near the start of each loop, then the eyes stay open.
const BLINK: Keyframe[] = [
	{ transform: "scaleY(1)" },
	{ transform: "scaleY(1)", offset: 0.12 },
	{ transform: "scaleY(0.1)", offset: 0.17 },
	{ transform: "scaleY(1)", offset: 0.2 },
	{ transform: "scaleY(0.1)", offset: 0.24 },
	{ transform: "scaleY(1)", offset: 0.27 },
	{ transform: "scaleY(1)" },
];

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
	let hovering = false;
	let dark = false;
	let settling = false;
	let blinking: Animation | undefined;

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

	const setEyes = (open: boolean) => {
		parts.svg.dataset.eyes = open ? "open" : "closed";
		parts.eyes.style.transform = open ? "scaleY(1)" : "scaleY(0)";
		blinking?.cancel();
		// jsdom has no Web Animations.
		blinking =
			open && typeof parts.eyes.animate === "function"
				? parts.eyes.animate(BLINK, {
						duration: timing.blinkEveryMs,
						iterations: Number.POSITIVE_INFINITY,
					})
				: undefined;
	};

	// Each frame tears the bands by a new amount and picks light or dark at
	// random, weighted toward the target, so the logo stutters before it lands.
	const glitch = async (toDark: boolean) => {
		parts.svg.dataset.state = "glitching";
		parts.screen.setAttribute("filter", `url(#${parts.glitchFilterId})`);
		parts.scanlines.style.opacity = "1";
		for (let i = 0; i < timing.frames; i++) {
			const progress = i / (timing.frames - 1);
			const tear = Math.sin(Math.PI * progress) * (22 + random() * 18);
			parts.turbulence.setAttribute(
				"seed",
				String(1 + Math.floor(random() * 999)),
			);
			parts.displacement.setAttribute("scale", tear.toFixed(1));
			const darkChance = toDark ? 0.25 + progress * 0.7 : 0.75 - progress * 0.7;
			parts.ghost.style.opacity = random() < darkChance ? "1" : "0";
			await sleep(timing.frameMs);
		}
		parts.screen.removeAttribute("filter");
		parts.scanlines.style.opacity = "0";
		parts.ghost.style.opacity = toDark ? "1" : "0";
		parts.svg.dataset.state = toDark ? "dark" : "light";
		dark = toDark;
	};

	// One loop moves the logo toward the hover state, so quick enters and leaves
	// queue up instead of overlapping.
	const settle = async () => {
		if (settling) {
			return;
		}
		settling = true;
		try {
			while (dark !== hovering) {
				if (hovering) {
					await glitch(true);
					await sleep(timing.eyesOpenAfterMs);
					if (hovering) {
						setEyes(true);
					}
				} else {
					setEyes(false);
					await sleep(timing.glitchOutAfterMs);
					await glitch(false);
				}
			}
		} finally {
			settling = false;
		}
	};

	return {
		hover: (on) => {
			hovering = on;
			// After destroy, pending sleeps reject with the abort reason.
			settle().catch((error) => {
				if (!signal.aborted) {
					throw error;
				}
			});
		},
		destroy: () => {
			abort.abort();
			blinking?.cancel();
		},
	};
};
