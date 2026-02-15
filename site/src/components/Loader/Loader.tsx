import { Spinner } from "components/Spinner/Spinner";
import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";

interface LoaderProps extends HTMLAttributes<HTMLDivElement> {
	fullscreen?: boolean;
	size?: "sm" | "lg";
	/**
	 * A label for the loader. This is used for accessibility purposes.
	 */
	label?: string;
}

export const Loader: FC<LoaderProps> = ({
	fullscreen,
	size = "lg",
	label = "Loading...",
	className,
	...attrs
}) => {
	return (
		<div
			data-testid="loader"
			{...attrs}
			className={cn(
				"flex items-center justify-center",
				fullscreen ? "absolute inset-0 bg-surface-primary" : "w-full p-8",
				className,
			)}
		>
			<Spinner aria-label={label} size={size} loading={true} />
		</div>
	);
};
