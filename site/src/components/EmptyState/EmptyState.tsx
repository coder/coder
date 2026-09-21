import { cn } from "cn";
import type { ComponentProps, FC, ReactNode } from "react";

export interface EmptyStateProps extends ComponentProps<"div"> {
	/** Text Message to display, placed inside Typography component */
	message: string;
	/** Longer optional description to display below the message */
	description?: string | ReactNode;
	cta?: ReactNode;
	image?: ReactNode;
	/**
	 * Optional icon rendered in a badge above the message. Provide it already
	 * sized and colored (e.g. `size-9 text-highlight-sky`); the badge wrapper is
	 * supplied by this component.
	 */
	icon?: ReactNode;
	isCompact?: boolean;
}

/**
 * Component to place on screens or in lists that have no content. Optionally
 * provide a button that would allow the user to return from where they were,
 * or to add an item that they currently have none of.
 */
export const EmptyState: FC<EmptyStateProps> = ({
	message,
	description,
	cta,
	image,
	icon,
	isCompact,
	className,
	...attrs
}) => {
	return (
		<div
			className={cn(
				"overflow-hidden flex flex-col justify-center items-center text-center min-h-96 py-20 px-10 relative",
				isCompact && "min-h-44 py-2.5",
				className,
			)}
			{...attrs}
		>
			{icon && (
				<div className="mb-4 flex size-12 items-center justify-center rounded-md bg-surface-sky">
					{icon}
				</div>
			)}
			<h5
				className={cn(
					"m-0 text-content-primary",
					icon ? "font-semibold text-sm" : "font-medium text-lg",
				)}
			>
				{message}
			</h5>
			{description && (
				<p
					className={cn(
						"max-w-md text-content-secondary text-sm",
						icon ? "mt-2 mb-0 max-w-[420px]" : "mt-4",
					)}
				>
					{description}
				</p>
			)}
			{cta && <div className={cn("mt-6", icon && "mt-4")}>{cta}</div>}
			{image}
		</div>
	);
};
