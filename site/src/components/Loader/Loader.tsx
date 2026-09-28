import { cn } from "cn";
import { Spinner } from "#/components/Spinner/Spinner";

type LoaderProps = React.ComponentProps<"div"> & {
	fullscreen?: boolean;
	size?: "sm" | "lg";
	/**
	 * A label for the loader. This is used for accessibility purposes.
	 */
	label?: string;
};

export const Loader: React.FC<LoaderProps> = ({
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
