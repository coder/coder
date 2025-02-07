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
			 * Indicates whether the `children` prop should be unmounted during
			 * a loading state. Defaults to `false` - unmounting HTML elements
			 * like form controls can lead to invalid HTML, so this prop should
			 * be used with care and only if it prevents render performance
			 * issues.
			 */
			unmountChildrenWhileLoading?: boolean;

			/**
			 * Specifies whether there should be a delay before the spinner
			 * appears on screen. If not specified, the spinner always appears
			 * immediately when `loading` is true.
			 *
			 * Can help avoid page flickering issues. Example:
			 * 1. You have a modal that takes a moment to close, and it has
			 *    Spinner content inside it.
			 * 2. The user triggers a loading transition, and you want to show
			 *    the spinner at some point, especially if the transition takes
			 *    long enough.
			 * 3. The problem is, if the spinner mounting and modal closing
			 *    happen in quick enough succession, the user will see a brief
			 *    spinner flicker before the modal closes, making the UI appear
			 *    janky, even though it is accurately modeling the state of the
			 *    application.
			 * 4. The solution, then, to keep the UI feeling polished, is to lie
			 *    to the user and only show the loading state if some
			 *    pre-determined period has lapsed.
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
	/**
	 * @todo Figure out if this conditional logic can ever cause a component to
	 * lose state when showSpinner flips from false to true while
	 * unmountedWhileLoading is false. I would hope not, since the children prop
	 * is the same in both cases, but I need to test this out
	 */
	const showSpinner = useShowSpinner(loading, spinnerDelayMs);
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

// Splitting off logic into custom hook so that we can abstract away the chaos
// of handling Spinner's re-render logic. The result is a simple boolean, but
// the steps to calculate that boolean accurately while avoiding re-render
// issues got a little heady
function useShowSpinner(loading: boolean, spinnerDelayMs: number): boolean {
	// Disallow negative timeout values and fractional values, but also round
	// the delay down if it's small enough that it might as well be immediate
	// from a user perspective
	let safeDelay = Math.trunc(spinnerDelayMs);
	if (safeDelay < 100) {
		safeDelay = 0;
	}

	// Doing a bunch of mid-render state syncs to minimize risks of UI tearing
	// during re-renders. It's ugly, but it's what the React team officially
	// recommends (even though this specific case is extra nasty).
	//
	// Be very careful with this logic; React only bails out of redundant state
	// updates if they happen outside of a render. Inside a render, if you keep
	// calling a state dispatch, you will get an infinite render loop, no matter
	// what value you dispatch. There must be a "base case" in the render path
	// that causes state dispatches to stop entirely so that React can move on
	// to the next component in the Fiber subtree
	const [delayLapsed, setDelayLapsed] = useState(safeDelay === 0);
	const [renderCache, setRenderCache] = useState({ loading, safeDelay });
	const canResetLapseOnRerender =
		delayLapsed && !loading && loading !== renderCache.loading;
	if (canResetLapseOnRerender) {
		setDelayLapsed(false);
		setRenderCache((current) => ({ ...current, loading }));
	}
	const delayWasRemovedOnRerender =
		!delayLapsed && safeDelay === 0 && renderCache.safeDelay !== safeDelay;
	if (delayWasRemovedOnRerender) {
		setDelayLapsed(true);
		setRenderCache((current) => ({ ...current, safeDelay }));
	}
	useEffect(() => {
		if (safeDelay === 0) {
			return;
		}
		const id = window.setTimeout(() => setDelayLapsed(true), safeDelay);
		return () => window.clearTimeout(id);
	}, [safeDelay]);

	return delayLapsed && loading;
}
