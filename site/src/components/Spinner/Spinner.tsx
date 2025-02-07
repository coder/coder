/**
 * @file This component was inspired by Radix's Spinner component and developed
 * using help from Vercel's V0.
 *
 * @see {@link https://www.radix-ui.com/themes/docs/components/spinner}
 * @see {@link https://v0.dev/}
 */

import isChromatic from "chromatic/isChromatic";
import { type VariantProps, cva } from "class-variance-authority";
import { type CSSProperties, type FC, useEffect, useState } from "react";
import { cn } from "utils/cn";

const SPINNER_LEAF_COUNT = 8;

const spinnerVariants = cva("", {
	variants: {
		size: {
			lg: "size-icon-lg",
			sm: "size-icon-sm",
		},
	},
	defaultVariants: {
		size: "lg",
	},
});

type SpinnerProps = Readonly<
	React.SVGProps<SVGSVGElement> &
		VariantProps<typeof spinnerVariants> & {
			loading: boolean;

			/**
			 * Indicates whether the children prop should be unmounted during
			 * a loading state. Defaults to false - unmounting HTML elements
			 * like form controls can lead to invalid HTML, so this prop should
			 * be used with care and only if it prevents render performance
			 * issues.
			 */
			unmountChildrenWhileLoading?: boolean;

			/**
			 * Specifies whether there should be a delay before the spinner
			 * appears on screen. If not specified, the spinner always appears
			 * immediately.
			 *
			 * Can help avoid page flickering issues. (e.g., You have a modal
			 * that takes a moment to close, and it has Spinner content inside
			 * it. The user triggers a loading transition, and you want to show
			 * the spinner at some point if a transition takes long enough, but
			 * if the spinner mounting and modal closing happen in too quick of
			 * a succession, the UI looks janky. So even though you might flip
			 * the loading state immediately, you want to wait a second to show
			 * it in case the modal can close quickly enough. It's lying to the
			 * user in a way that makes the UI feel more polished.)
			 */
			spinnerDelayMs?: number;
		}
>;

const leafIndices = Array.from({ length: SPINNER_LEAF_COUNT }, (_, i) => i);
const animationSettings: CSSProperties = isChromatic()
	? {}
	: {
			transitionDuration: `${0.1 * SPINNER_LEAF_COUNT}s`,
			transitionTimingFunction: "ease-in-out",
			animationIterationCount: "infinite",
		};

export const Spinner: FC<SpinnerProps> = ({
	className,
	size,
	loading,
	children,
	spinnerDelayMs = 0,
	unmountChildrenWhileLoading = false,
	...delegatedProps
}) => {
	// Disallow negative timeout values and fractional values, but also round
	// the delay down if it's small enough that it might as well be immediate
	// from a user perspective
	let safeDelay = Math.trunc(spinnerDelayMs);
	if (safeDelay < 100) {
		safeDelay = 0;
	}

	// Doing a bunch of mid-render state syncs to minimize risks of
	// contradictory states during re-renders. It's ugly, but it's what the
	// React team officially recommends. Be very careful with this logic; React
	// only bails out of redundant state updates if they happen outside of a
	// render. Inside a render, if you keep calling a state dispatch, you will
	// get an infinite render loop, no matter what the state value is.
	const [cachedDelay, setCachedDelay] = useState(safeDelay);
	const [delayLapsed, setDelayLapsed] = useState(safeDelay === 0);
	if (delayLapsed && !loading) {
		setDelayLapsed(false);
	}
	const delayWasRemovedOnRerender =
		!delayLapsed && safeDelay === 0 && cachedDelay !== safeDelay;
	if (delayWasRemovedOnRerender) {
		setDelayLapsed(true);
		setCachedDelay(safeDelay);
	}
	useEffect(() => {
		if (safeDelay === 0) {
			return;
		}
		const id = window.setTimeout(() => setDelayLapsed(true), safeDelay);
		return () => window.clearTimeout(id);
	}, [safeDelay]);

	/**
	 * @todo Figure out if this conditional logic can ever cause a component to
	 * lose state when showSpinner flips from false to true while
	 * unmountedWhileLoading is false. I would hope not, since the children prop
	 * is the same in both cases, but I need to test this out
	 */
	const showSpinner = loading && delayLapsed;
	if (!showSpinner) {
		return children;
	}

	return (
		<>
			<svg
				// Fill is the only prop that should be allowed to be
				// overridden; all other props must come after destructuring
				fill="currentColor"
				{...delegatedProps}
				viewBox="0 0 24 24"
				xmlns="http://www.w3.org/2000/svg"
				className={cn(className, spinnerVariants({ size }))}
			>
				<title>Loading&hellip;</title>
				{leafIndices.map((index) => (
					<rect
						key={index}
						x="10.9"
						y="2"
						width="2"
						height="5.5"
						rx="1"
						style={{
							...animationSettings,
							transform: `rotate(${index * (360 / SPINNER_LEAF_COUNT)}deg)`,
							transformOrigin: "center",
							animationDelay: `${-index * 0.1}s`,
						}}
					/>
				))}
			</svg>

			{!unmountChildrenWhileLoading && (
				<div className="sr-only">
					This content is loading:
					{children}
				</div>
			)}
		</>
	);
};
