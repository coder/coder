import { cn } from "cn";
import { Slot } from "radix-ui";

type TranscriptRowProps = React.ComponentProps<"div"> & {
	asChild?: boolean;
};

/** Consistent min-height for transcript rows that bypass the ToolCall primitives. */
export const TranscriptRow: React.FC<TranscriptRowProps> = ({
	asChild = false,
	className,
	...props
}) => {
	const Comp = asChild ? Slot.Root : "div";

	return (
		<Comp {...props} className={cn("flex min-h-6 items-center", className)} />
	);
};
