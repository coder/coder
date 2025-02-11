/**
 * @file This component was inspired by Radix's Spinner component. The animation
 * settings were developed using Vercel's v0.
 *
 * @see {@link https://www.radix-ui.com/themes/docs/components/spinner}
 * @see {@link https://v0.dev/}
 */

import isChromatic from "chromatic/isChromatic";
import { type VariantProps, cva } from "class-variance-authority";
import { type CSSProperties, type FC, useEffect, useState } from "react";
import { cn } from "utils/cn";

const SPINNER_LEAF_COUNT = 8;
const MAX_SPINNER_DELAY_MS = 2_000;

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
			 * a loading state. Defaults to `false` - fully unmounting a child
			 * component has two main risks:
			 * 1. Hiding children can create invalid HTML. (For example, if you
			 *    have an HTML input and a label, but only hide the input behind
			 *    the spinner during loading state, the label becomes
			 *    "detached"). This not only breaks behavior for screen readers
			 *    but can also create nasty undefined behavior for some built-in
			 *    HTML elements.
			 * 2. Unmounting a component will cause any of its internal state to
			 *    be completely wiped. Unless the component has all of its state
			 *    controlled by a parent or external state management tool, the
			 *    component will have all its initial state once the loading
			 *    transition ends.
			 *
			 * If you do need reset all the state after a loading transition
			 * and you can't unmount the component without creating invalid
			 * HTML, use a render key to reset the state.
			 * @see {@link https://react.dev/learn/you-might-not-need-an-effect#resetting-all-state-when-a-prop-changes}
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
	// Disallow negative timeout values and fractional values, but also round
	// the delay down if it's small enough that it might as well be immediate
	// from a user perspective
	let safeDelay = Number.isNaN(spinnerDelayMs)
		? 0
		: Math.min(MAX_SPINNER_DELAY_MS, Math.trunc(spinnerDelayMs));
	if (safeDelay < 100) {
		safeDelay = 0;
	}
	/**
	 * Doing a bunch of mid-render state syncs to minimize risks of UI tearing
	 * during re-renders. It's ugly, but it's what the React team officially
	 * recommends.
	 * @see {@link https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes}
	 */
	const [delayLapsed, setDelayLapsed] = useState(safeDelay === 0);
	const canResetLapseOnRerender = delayLapsed && !loading;
	if (canResetLapseOnRerender) {
		setDelayLapsed(false);
	}
	// Have to cache delay so that we don't "ping-pong" between state syncs for
	// the delayLapsed state and accidentally create an infinite render loop
	const [cachedDelay, setCachedDelay] = useState(safeDelay);
	const delayWasRemovedOnRerender =
		!delayLapsed && safeDelay === 0 && safeDelay !== cachedDelay;
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
	const showSpinner = delayLapsed && loading;

	// Conditional rendering logic is more convoluted than normal because we
	// need to make sure that the children prop is always placed in the same JSX
	// "slot" by default, no matter the value of `loading`. Even if the children
	// prop value is exactly the same each time, the state for the associated
	// component will get wiped if the parent changes
	return (
		<>
			{showSpinner && (
				<svg
					// `fill` is the only prop that should be allowed to be
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
			)}

			{/*
			 * Invert the condition (showSpinner && unmountChildrenWhileLoading)
			 * (which is the only one that should result in fully-unmounted
			 * content), and then if we still get content, handle the other
			 * three cases of the boolean truth table more granularly
			 */}
			{(!showSpinner || !unmountChildrenWhileLoading) && (
				<>
					{showSpinner && (
						<span className="sr-only">This content is loading: </span>
					)}
					<span className={showSpinner ? "sr-only" : "inline"}>{children}</span>
				</>
			)}
		</>
	);
};
