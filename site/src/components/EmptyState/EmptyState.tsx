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
	const Heading = image ? "h2" : "h5";

	return (
		<div
			className={cn(
				"overflow-hidden flex flex-col items-center text-center min-h-96 px-10 relative",
				image ? "py-12 gap-12" : "justify-center py-20",
				isCompact && "min-h-44 py-2.5",
				className,
			)}
			{...attrs}
		>
			{image}
			<div
				className={
					image
						? "flex flex-col gap-4 justify-center items-center max-w-105"
						: "contents"
				}
			>
				<div className={image ? "flex flex-col gap-2" : "contents"}>
					<Heading
						className={cn(
							"m-0 text-content-primary text-lg",
							image ? "font-semibold" : "font-medium",
						)}
					>
						{message}
					</Heading>
					{description && (
						<p
							className={cn(
								"max-w-md text-content-secondary text-sm",
								image ? "m-0 font-normal" : "mt-4",
							)}
						>
							{description}
						</p>
					)}
				</div>
				{cta && <div className={image ? undefined : "mt-6"}>{cta}</div>}
			</div>
		</div>
	);
};
