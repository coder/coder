/**
 * This component was inspired by
 * https://www.radix-ui.com/themes/docs/components/spinner and developed using
 * https://v0.dev/ help.
 */

import isChromatic from "chromatic/isChromatic";
import { type VariantProps, cva } from "class-variance-authority";
import type { FC, ReactNode } from "react";
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
			children?: ReactNode;
			loading?: boolean;
			unmountedWhileLoading?: boolean;
			spinnerStartDelayMs?: number;
		}
>;

const leavesIterable = Array.from({ length: SPINNER_LEAF_COUNT }, (_, i) => i);

export const Spinner: FC<SpinnerProps> = ({
	className,
	size,
	loading,
	children,
	...props
}) => {
	if (!loading) {
		return children;
	}

	return (
		<svg
			viewBox="0 0 24 24"
			xmlns="http://www.w3.org/2000/svg"
			fill="currentColor"
			className={cn(className, spinnerVariants({ size }))}
			{...props}
		>
			<title>Loading spinner</title>
			{leavesIterable.map((leafIndex) => (
				<rect
					key={leafIndex}
					x="10.9"
					y="2"
					width="2"
					height="5.5"
					rx="1"
					// 0.8 = leaves * 0.1
					className={
						isChromatic() ? "" : "animate-[loading_0.8s_ease-in-out_infinite]"
					}
					style={{
						transform: `rotate(${leafIndex * (360 / SPINNER_LEAF_COUNT)}deg)`,
						transformOrigin: "center",
						animationDelay: `${-leafIndex * 0.1}s`,
					}}
				/>
			))}
		</svg>
	);
};
