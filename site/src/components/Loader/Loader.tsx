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
	...attrs
}) => {
	return (
		<div
			{...attrs}
			className={cn(
				fullscreen && classNames.fullscreen,
				!fullscreen && classNames.inline,
				attrs.className,
			)}
			data-testid="loader"
		>
			<Spinner aria-label={label} size={size} loading={true} />
		</div>
	);
};

const classNames = {
	inline: "p-8 w-full flex items-center justify-center",
	fullscreen:
		"absolute inset-0 flex justify-center items-center bg-content-primary",
};
