/**
 * Copied from shadcn/ui on 06/20/2025.
 * @see {@link https://ui.shadcn.com/docs/components/skeleton}
 */
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "#/utils/cn";

const skeletonVariants = cva("bg-surface-tertiary animate-pulse", {
	variants: {
		variant: {
			default: "rounded-md",
			text: "rounded-full h-2 my-1",
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

export const Skeleton: React.FC<SkeletonProps> = ({
	className,
	variant,
	width,
	height,
	style,
	...props
}) => {
	return (
		<div
			data-slot="skeleton"
			className={cn(skeletonVariants({ variant }), className)}
			style={{ width, height, ...style }}
			{...props}
		/>
	);
};
