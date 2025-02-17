/**
 * This component was inspired by
 * https://www.radix-ui.com/themes/docs/components/spinner and developed using
 * https://v0.dev/ help.
 */

import isChromatic from "chromatic/isChromatic";
import { type VariantProps, cva } from "class-variance-authority";
import type { ReactNode } from "react";
import { cn } from "utils/cn";

const leaves = 8;

const spinnerVariants = cva("", {
	variants: {
		size: {
			lg: "size-icon-md",
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
			className={cn(spinnerVariants({ size, className }))}
			{...props}
		>
			<title>Loading spinner</title>
			{[...Array(leaves)].map((_, i) => {
				const rotation = i * (360 / leaves);

				return (
					<rect
						key={i}
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
							transform: `rotate(${rotation}deg)`,
							transformOrigin: "center",
							animationDelay: `${-i * 0.1}s`,
						}}
					/>
				);
			})}
		</svg>
	);
}
