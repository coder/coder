/**
 * Loader component following Coder's established patterns
 * Uses Tailwind classes with cva for consistent variant management
 */
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { Spinner } from "components/Spinner/Spinner";
import { forwardRef } from "react";
import { cn } from "utils/cn";

const loaderVariants = cva(
	"flex items-center justify-center",
	{
		variants: {
			variant: {
				default: "w-full",
				fullscreen: "absolute inset-0 bg-surface-primary",
				inline: "w-full",
				compact: "w-auto",
			},
			size: {
				sm: "p-4",
				md: "p-8",
				lg: "p-12",
			},
		},
		compoundVariants: [
			{
				variant: "fullscreen",
				size: ["sm", "md", "lg"],
				class: "p-0", // Fullscreen doesn't need padding
			},
			{
				variant: "compact",
				size: ["sm", "md", "lg"],
				class: "p-2", // Compact uses minimal padding
			},
		],
		defaultVariants: {
			variant: "default",
			size: "md",
		},
	},
);

export interface LoaderProps
	extends React.HTMLAttributes<HTMLDivElement>,
		VariantProps<typeof loaderVariants> {
	/**
	 * A label for the loader. This is used for accessibility purposes.
	 */
	label?: string;
	/**
	 * Size of the spinner itself
	 */
	spinnerSize?: "sm" | "lg";
	/**
	 * Whether the loader is currently loading
	 */
	loading?: boolean;
	/**
	 * Content to show when not loading
	 */
	children?: React.ReactNode;
	/**
	 * Render as a different element
	 */
	asChild?: boolean;
}

export const Loader = forwardRef<HTMLDivElement, LoaderProps>(
	({
		variant = "default",
		size = "md",
		label = "Loading...",
		spinnerSize = "lg",
		loading = true,
		children,
		className,
		asChild = false,
		...props
	}, ref) => {
		const Comp = asChild ? Slot : "div";

		// If not loading and has children, show children
		if (!loading && children) {
			return <>{children}</>;
		}

		// If not loading and no children, don't render anything
		if (!loading) {
			return null;
		}

		return (
			<Comp
				ref={ref}
				className={cn(loaderVariants({ variant, size }), className)}
				data-testid="loader"
				{...props}
			>
				<Spinner aria-label={label} size={spinnerSize} loading={true} />
			</Comp>
		);
	},
);

Loader.displayName = "Loader";

// Legacy compatibility helpers - can be removed later
export interface LegacyLoaderProps {
	fullscreen?: boolean;
	size?: "sm" | "lg";
	label?: string;
}

// Helper function for migration - can be removed later
export const mapLegacyLoaderProps = (props: LegacyLoaderProps): Partial<LoaderProps> => {
	return {
		variant: props.fullscreen ? "fullscreen" : "default",
		spinnerSize: props.size || "lg",
		label: props.label,
	};
};
