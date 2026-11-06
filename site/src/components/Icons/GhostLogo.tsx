import { cn } from "cn";
import { useEffect, useId, useRef } from "react";
import { CODER_LOGO_PATH } from "./coderLogoPath";
import {
	createGhostLogoController,
	type GhostLogoController,
} from "./ghostLogoController";

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";

type GhostLogoProps = {
	className?: string;
};

/**
 * The Coder logo with a hover easter egg: it glitches into a backlit
 * silhouette with blinking eyes, and glitches back when the pointer leaves.
 * It stays plain for reduced motion.
 */
export const GhostLogo: React.FC<GhostLogoProps> = ({ className }) => {
	// SVG url(#…) references need an id without React's punctuation.
	const id = useId().replace(/[^\w-]/g, "");
	const glitchFilterId = `ghost-glitch-${id}`;
	const backlightFilterId = `ghost-backlight-${id}`;
	const scanlinesId = `ghost-scanlines-${id}`;

	const controller = useRef<GhostLogoController | null>(null);
	const svg = useRef<SVGSVGElement>(null);
	const screen = useRef<SVGGElement>(null);
	const turbulence = useRef<SVGFETurbulenceElement>(null);
	const displacement = useRef<SVGFEDisplacementMapElement>(null);
	const ghost = useRef<SVGGElement>(null);
	const eyes = useRef<SVGGElement>(null);
	const scanlines = useRef<SVGGElement>(null);

	useEffect(() => {
		// jsdom has no matchMedia.
		if (window.matchMedia?.(REDUCED_MOTION).matches) {
			return;
		}
		const parts = {
			svg: svg.current,
			screen: screen.current,
			turbulence: turbulence.current,
			displacement: displacement.current,
			ghost: ghost.current,
			eyes: eyes.current,
			scanlines: scanlines.current,
		};
		if (
			!parts.svg ||
			!parts.screen ||
			!parts.turbulence ||
			!parts.displacement ||
			!parts.ghost ||
			!parts.eyes ||
			!parts.scanlines
		) {
			return;
		}
		const ghostLogoController = createGhostLogoController({
			svg: parts.svg,
			screen: parts.screen,
			turbulence: parts.turbulence,
			displacement: parts.displacement,
			ghost: parts.ghost,
			eyes: parts.eyes,
			scanlines: parts.scanlines,
			glitchFilterId,
		});
		controller.current = ghostLogoController;
		return () => {
			ghostLogoController.destroy();
			controller.current = null;
		};
	}, [glitchFilterId]);

	return (
		<svg
			ref={svg}
			fill="currentColor"
			viewBox="0 0 120 60"
			xmlns="http://www.w3.org/2000/svg"
			className={cn(
				"h-7 aspect-square text-content-primary overflow-visible",
				className,
			)}
			data-state="light"
			data-eyes="closed"
			onMouseEnter={() => controller.current?.hover(true)}
			onMouseLeave={() => controller.current?.hover(false)}
		>
			<title>Coder logo</title>
			<defs>
				{/* Two blurs of the silhouette: a tight glow plus a wide one. */}
				<filter
					id={backlightFilterId}
					x="-100%"
					y="-100%"
					width="300%"
					height="300%"
				>
					<feGaussianBlur in="SourceGraphic" stdDeviation="5" result="near" />
					<feGaussianBlur in="SourceGraphic" stdDeviation="14" result="far" />
					<feMerge>
						<feMergeNode in="far" />
						<feMergeNode in="near" />
					</feMerge>
				</filter>
				{/* TV tearing: noise that varies only down the logo shifts horizontal bands
				    sideways. G is pinned at 0.5, so nothing moves vertically. */}
				<filter
					id={glitchFilterId}
					x="-60%"
					y="-120%"
					width="220%"
					height="340%"
					colorInterpolationFilters="sRGB"
				>
					<feTurbulence
						ref={turbulence}
						type="fractalNoise"
						baseFrequency="0.0001 0.12"
						numOctaves={2}
						seed={1}
						result="noise"
					/>
					<feColorMatrix
						in="noise"
						type="matrix"
						values="1.6 0 0 0 -0.3  0 0 0 0 0.5  0 0 0 0 0  0 0 0 0 1"
						result="shift"
					/>
					<feDisplacementMap
						ref={displacement}
						in="SourceGraphic"
						in2="shift"
						scale={0}
						xChannelSelector="R"
						yChannelSelector="G"
					/>
				</filter>
				<pattern
					id={scanlinesId}
					width="8"
					height="3.2"
					patternUnits="userSpaceOnUse"
				>
					<rect
						width="8"
						height="1.3"
						className="fill-surface-primary"
						opacity={0.75}
					/>
				</pattern>
			</defs>
			<g ref={screen}>
				<path d={CODER_LOGO_PATH} />
				<g ref={ghost} style={{ opacity: 0 }}>
					<path
						d={CODER_LOGO_PATH}
						stroke="currentColor"
						strokeWidth={4}
						opacity={0.9}
						filter={`url(#${backlightFilterId})`}
					/>
					{/* The stroke grows the silhouette by 2 units on each side, under half
					    a pixel in the navbar, so the dark logo doesn't read thinner. */}
					<path
						d={CODER_LOGO_PATH}
						className="fill-surface-primary stroke-surface-primary"
						strokeWidth={4}
						strokeLinejoin="round"
					/>
					<g
						ref={eyes}
						className="[transform-box:fill-box] origin-center transition-transform duration-100"
						style={{ transform: "scaleY(0)" }}
					>
						<rect x="90" y="8" width="6.5" height="15" />
						<rect x="102.5" y="8" width="6.5" height="15" />
					</g>
				</g>
				<g ref={scanlines} pointerEvents="none" style={{ opacity: 0 }}>
					<rect
						x="-4"
						y="-2"
						width="128"
						height="64"
						fill={`url(#${scanlinesId})`}
					/>
				</g>
			</g>
		</svg>
	);
};
