import { Slot } from "radix-ui";
import type { ComponentPropsWithRef, FC } from "react";
import { cn } from "#/utils/cn";

type TranscriptRowProps = ComponentPropsWithRef<"div"> & {
	asChild?: boolean;
};

/** Consistent min-height for transcript rows that bypass the ToolCall primitives. */
export const TranscriptRow: FC<TranscriptRowProps> = ({
	asChild = false,
	className,
	...props
}) => {
	const Comp = asChild ? Slot.Root : "div";

	return (
		<Comp {...props} className={cn("flex min-h-6 items-center", className)} />
	);
};
