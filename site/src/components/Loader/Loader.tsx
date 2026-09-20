import { cn } from "cn";
import type { ComponentProps, FC } from "react";
import { Spinner } from "#/components/Spinner/Spinner";

interface LoaderProps extends ComponentProps<"div"> {
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
	label,
	className,
	...attrs
}) => {
	const resolvedLabel = label ?? "Loading";

	return (
		<div
			{...attrs}
			role="status"
			aria-live="polite"
			aria-label={resolvedLabel}
			data-testid="loader"
			className={cn(
				"flex items-center justify-center",
				fullscreen ? "absolute inset-0 bg-surface-primary" : "w-full p-8",
				className,
			)}
		>
			<Spinner size={size} loading />
		</div>
	);
};
