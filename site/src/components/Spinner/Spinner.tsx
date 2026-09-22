/**
 * This component was inspired by
 * https://www.radix-ui.com/themes/docs/components/spinner and developed using
 * https://v0.dev/ help.
 */

import { isPixel } from "@coder/pixel-storybook/storyapi";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import type { ReactNode } from "react";

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

type SpinnerProps = React.ComponentProps<"svg"> &
	VariantProps<typeof spinnerVariants> & {
		children?: ReactNode;
		loading?: boolean;
		/**
		 * Exposes the spinner as an accessible live region labelled with this text. Leave undefined for
		 * decorative spinners, e.g. inside a component that already provides its own status region.
		 */
		label?: string;
	};

export function Spinner({
	className,
	size,
	loading,
	children,
	label,
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
			role={label ? "status" : undefined}
			aria-label={label}
			className={cn(spinnerVariants({ size, className }))}
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
					className={isPixel() ? "" : "animate-spinner-leaf"}
					style={{
						transform: `rotate(${leaf * (360 / leaves.length)}deg)`,
						transformOrigin: "center",
						"--spinner-leaf-index": leaf,
						"--spinner-leaf-count": leaves.length,
					}}
				/>
			))}
		</svg>
	);
}
