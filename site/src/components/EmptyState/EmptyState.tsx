import { cn } from "cn";

export type EmptyStateProps = React.ComponentProps<"div"> & {
	/** Text Message to display, placed inside Typography component */
	message: string;
	/** Longer optional description to display below the message */
	description?: string | React.ReactNode;
	cta?: React.ReactNode;
	image?: React.ReactNode;
	isCompact?: boolean;
};

/**
 * Component to place on screens or in lists that have no content. Optionally
 * provide a button that would allow the user to return from where they were,
 * or to add an item that they currently have none of.
 */
export const EmptyState: React.FC<EmptyStateProps> = ({
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
				"overflow-hidden flex flex-col items-center text-center min-h-96 py-12 px-10 relative gap-12",
				isCompact && "min-h-44 py-2.5",
				className,
			)}
			{...attrs}
		>
			{image}
			<div className="flex flex-col gap-4 justify-center items-center max-w-105">
				<div className="flex flex-col gap-2">
					<h2 className="m-0 font-semibold text-content-primary text-lg">
						{message}
					</h2>
					{description && (
						<p className="m-0 max-w-md text-content-secondary text-sm font-normal">
							{description}
						</p>
					)}
				</div>
				{cta && <div>{cta}</div>}
			</div>
		</div>
	);
};
