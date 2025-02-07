/**
 * @file This component was inspired by Radix's Spinner component and developed
 * using help from Vercel's V0.
 *
 * @see {@link https://www.radix-ui.com/themes/docs/components/spinner}
 * @se {@link https://v0.dev/}
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
			unmountedWhileLoading?: boolean;

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
			spinnerStartDelayMs?: number;
		}
>;

const leavesIterable = Array.from({ length: SPINNER_LEAF_COUNT }, (_, i) => i);
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
	spinnerStartDelayMs = 0,
	unmountedWhileLoading = false,
	...delegatedProps
}) => {
	/**
	 * @todo Figure out if this conditional logic causes a component to lose
	 * state. I would hope not, since the children prop is the same in both
	 * cases, but I need to test this out
	 */
	const showSpinner = useShowSpinner(loading, spinnerStartDelayMs);
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
				{leavesIterable.map((leafIndex) => (
					<rect
						key={leafIndex}
						x="10.9"
						y="2"
						width="2"
						height="5.5"
						rx="1"
						style={{
							...animationSettings,
							transform: `rotate(${leafIndex * (360 / SPINNER_LEAF_COUNT)}deg)`,
							transformOrigin: "center",
							animationDelay: `${-leafIndex * 0.1}s`,
						}}
					/>
				))}
			</svg>

			{!unmountedWhileLoading && (
				<div className="sr-only">
					This content is loading:
					{children}
				</div>
			)}
		</>
	);
};

// Not a big fan of one-time custom hooks, but it helps insulate the main
// component from the chaos of handling all these state syncs, when the ultimate
// result is a simple boolean. A lot of browsers will be able to inline the
// function definition in some cases anyway
function useShowSpinner(
	loading: boolean,
	spinnerStartDelayMs: number,
): boolean {
	// Doing a bunch of mid-render state syncs to minimize risks of
	// contradictory states during re-renders. It's ugly, but it's what the
	// React team officially recommends. Be very careful with this logic; React
	// only bails out of redundant state updates if they happen outside of a
	// render. Inside a render, if you keep calling a state dispatch, you will
	// get an infinite render loop, no matter what the state value is.
	const [spinnerActive, setSpinnerActive] = useState(spinnerStartDelayMs === 0);
	const unmountImmediatelyOnRerender = spinnerActive && !loading;
	if (unmountImmediatelyOnRerender) {
		setSpinnerActive(false);
	}
	const mountImmediatelyOnRerender =
		!spinnerActive && loading && spinnerStartDelayMs === 0;
	if (mountImmediatelyOnRerender) {
		setSpinnerActive(true);
	}
	useEffect(() => {
		if (spinnerStartDelayMs === 0) {
			return;
		}

		const delayId = window.setTimeout(() => {
			setSpinnerActive(true);
		}, spinnerStartDelayMs);
		return () => window.clearTimeout(delayId);
	}, [spinnerStartDelayMs]);

	return spinnerActive && loading;
}
