import type { ComponentPropsWithRef, FC } from "react";
import { cn } from "#/utils/cn";

type InputProps = ComponentPropsWithRef<"input">;

export const Input: FC<InputProps> = ({ className, type, ...props }) => {
	return (
		<input
			type={type}
			className={cn(
				`flex h-10 w-full rounded-md border border-border border-solid bg-transparent px-3
				text-base shadow-xs transition-colors
				file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-content-primary
				placeholder:text-content-secondary
				focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link
				disabled:cursor-not-allowed disabled:opacity-50 md:text-sm text-inherit
				aria-invalid:border-border-destructive
				`,
				className,
			)}
			{...props}
		/>
	);
};
