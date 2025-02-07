import { type VariantProps, cva } from "class-variance-authority";
import { type FC, createContext, useContext } from "react";
import { cn } from "utils/cn";

const statusIndicatorVariants = cva(
	"font-medium inline-flex items-center gap-2",
	{
		variants: {
			variant: {
				success: "text-content-success",
				failed: "text-content-destructive",
				inactive: "text-highlight-grey",
				warning: "text-content-warning",
				pending: "text-highlight-sky",
			},
			size: {
				sm: "text-xs",
				md: "text-sm",
			},
		},
		defaultVariants: {
			variant: "success",
			size: "md",
		},
	},
);

type StatusIndicatorContextValue = VariantProps<typeof statusIndicatorVariants>;

const StatusIndicatorContext = createContext<StatusIndicatorContextValue>({});

export interface StatusIndicatorProps
	extends React.HTMLAttributes<HTMLDivElement>,
		StatusIndicatorContextValue {}

export const StatusIndicator: FC<StatusIndicatorProps> = ({
	size,
	variant,
	className,
	...props
}) => {
	return (
		<StatusIndicatorContext.Provider value={{ size, variant }}>
			<div
				className={cn(statusIndicatorVariants({ variant, size }), className)}
				{...props}
			/>
		</StatusIndicatorContext.Provider>
	);
};

const dotVariants = cva("rounded-full inline-block border-4 border-solid", {
	variants: {
		variant: {
			success: "bg-content-success border-surface-green",
			failed: "bg-content-destructive border-surface-destructive",
			inactive: "bg-highlight-grey border-surface-grey",
			warning: "bg-content-warning border-surface-orange",
			pending: "bg-highlight-sky border-surface-sky",
		},
		size: {
			sm: "size-3 border-4",
			md: "size-4 border-4",
		},
	},
	defaultVariants: {
		variant: "success",
		size: "md",
	},
});

export interface StatusIndicatorDotProps
	extends React.HTMLAttributes<HTMLDivElement>,
		VariantProps<typeof dotVariants> {}

export const StatusIndicatorDot: FC<StatusIndicatorDotProps> = ({
	className,
	// We allow the size and variant to be overridden directly by the component.
	// This allows StatusIndicatorDot to be used alone.
	size,
	variant,
	...props
}) => {
	const { size: ctxSize, variant: ctxVariant } = useContext(
		StatusIndicatorContext,
	);

	return (
		<div
			className={cn(
				dotVariants({ variant: variant ?? ctxVariant, size: size ?? ctxSize }),
				className,
			)}
			{...props}
		/>
	);
};
