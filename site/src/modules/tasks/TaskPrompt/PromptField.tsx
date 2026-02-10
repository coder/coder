import type { ComponentProps, FC } from "react";
import { cn } from "utils/cn";

const PromptField: FC<ComponentProps<"div">> = ({ className, ...props }) => {
	return (
		<div
			className={cn(
				"min-w-0 w-full md:w-auto md:max-w-[33%] flex flex-col gap-2 md:gap-0",
				className,
			)}
			{...props}
		/>
	);
};

const PromptFieldLabel: FC<ComponentProps<"label">> = ({
	className,
	...props
}) => {
	return (
		// biome-ignore lint/a11y/noLabelWithoutControl: htmlFor is passed via props
		<label
			className={cn(
				"text-xs font-medium text-content-secondary pl-4 md:sr-only",
				className,
			)}
			{...props}
		/>
	);
};

export { PromptField, PromptFieldLabel };
