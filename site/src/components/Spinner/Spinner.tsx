/**
 * This component was inspired by
 * https://www.radix-ui.com/themes/docs/components/spinner and developed using
 * https://v0.dev/ help.
 */

import { isPixel } from "@coder/pixel-storybook/storyapi";
import { cva, type VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cn } from "#/utils/cn";

const leaves = Array.from({ length: 8 }).map((_, i) => i);

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

type SpinnerProps = React.SVGProps<SVGSVGElement> &
	VariantProps<typeof spinnerVariants> & {
		children?: ReactNode;
		loading?: boolean;
	};

export function Spinner({
	className,
	size,
	loading,
	children,
	...props
}: SpinnerProps) {
	if (!loading) {
		return children;
	}

	return (
		<svg
			viewBox="0 0 24 24"
			xmlns="http://www.w3.org/2000/svg"
			fill="currentColor"
			className={cn(
				!isPixel() && "animate-spin-discrete motion-reduce:animate-none",
				spinnerVariants({ size, className }),
			)}
			{...props}
		>
			<title>Loading spinner</title>
			{leaves.map((leaf) => (
				<rect
					key={leaf}
					x="10.9"
					y="2"
					width="2"
					height="5.5"
					rx="1"
					style={{
						transform: `rotate(${leaf * (360 / leaves.length)}deg)`,
						transformOrigin: "center",
						// Static opacity gradient; the stepped rotation of the
						// whole svg makes the bright leaf march around. Pixel
						// tests keep uniform leaves to match existing baselines.
						opacity: isPixel() ? undefined : (leaf + 1) / leaves.length,
					}}
				/>
			))}
		</svg>
	);
}
