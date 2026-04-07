/**
 * Copied from shadcn/ui on 06/20/2025, extended with shape variants and
 * width/height props to ease migration from MUI Skeleton.
 * @see {@link https://ui.shadcn.com/docs/components/skeleton}
 */
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "#/utils/cn";

const skeletonVariants = cva("bg-surface-tertiary animate-pulse", {
	variants: {
		variant: {
			default: "rounded-md",
			text: "rounded",
			circular: "rounded-full",
		},
	},
	defaultVariants: {
		variant: "default",
	},
});

type SkeletonProps = React.ComponentProps<"div"> &
	VariantProps<typeof skeletonVariants> & {
		/** Width in pixels (number) or any CSS value (string). */
		width?: number | string;
		/** Height in pixels (number) or any CSS value (string). */
		height?: number | string;
	};

function Skeleton({
	className,
	variant,
	width,
	height,
	style,
	...props
}: SkeletonProps) {
	return (
		<div
			data-slot="skeleton"
			className={cn(skeletonVariants({ variant }), className)}
			style={{
				width: typeof width === "number" ? `${width}px` : width,
				height: typeof height === "number" ? `${height}px` : height,
				...style,
			}}
			{...props}
		/>
	);
}

export { Skeleton };
