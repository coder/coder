import type { ComponentProps, FC } from "react";
import { Stack } from "#/components/Stack/Stack";
import { cn } from "#/utils/cn";

/**
 * Use these components as the label in FormControlLabel when implementing radio
 * buttons, checkboxes, or switches to ensure proper styling.
 */

export const StackLabel: FC<ComponentProps<typeof Stack>> = ({
	className,
	...props
}) => {
	return (
		<Stack
			spacing={0.5}
			className={cn("pl-3 font-medium", className)}
			{...props}
		/>
	);
};

export const StackLabelHelperText: FC<ComponentProps<"p">> = ({
	className,
	...props
}) => {
	return (
		<p
			className={cn(
				"mt-0 text-xs text-content-secondary leading-[1.66] [&_strong]:text-content-primary",
				className,
			)}
			{...props}
		/>
	);
};
