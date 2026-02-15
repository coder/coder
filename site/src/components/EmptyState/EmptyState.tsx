import type { LucideIcon } from "lucide-react";
import type { FC, HTMLAttributes, ReactNode } from "react";
import { cn } from "utils/cn";

export interface EmptyStateProps extends HTMLAttributes<HTMLDivElement> {
	icon?: LucideIcon;
	/** Text Message to display, placed inside Typography component */
	message: string;
	/** Longer optional description to display below the message */
	description?: string | ReactNode;
	cta?: ReactNode;
	image?: ReactNode;
	isCompact?: boolean;
}

/**
 * Component to place on screens or in lists that have no content. Optionally
 * provide a button that would allow the user to return from where they were,
 * or to add an item that they currently have none of.
 */
export const EmptyState: FC<EmptyStateProps> = ({
	icon: Icon,
	message,
	description,
	cta,
	image,
	isCompact,
	className,
	...attrs
}) => {
	return (
		<div
			className={cn(
				"overflow-hidden flex flex-col gap-2 justify-center items-center text-center min-h-96 py-20 px-10 relative",
				isCompact && "min-h-44 py-2.5",
				className,
			)}
			{...attrs}
		>
			{Icon && <Icon className="size-icon-lg" />}
			<h5 className="text-xl font-medium m-0">{message}</h5>
			{description && (
				<p className="m-0 max-w-md text-content-secondary">{description}</p>
			)}
			{cta && <div className="mt-6">{cta}</div>}
			{image}
		</div>
	);
};
