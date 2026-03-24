import type { ComponentPropsWithRef } from "react";
import { cn } from "utils/cn";

type MessageProps = ComponentPropsWithRef<"div">;

export const Message = ({ className, ref, ...props }: MessageProps) => {
	return (
		<div ref={ref} className={cn("max-w-full min-w-0", className)} {...props} />
	);
};

type MessageContentProps = ComponentPropsWithRef<"div">;

export const MessageContent = ({
	className,
	ref,
	...props
}: MessageContentProps) => {
	return (
		<div
			ref={ref}
			className={cn(
				"whitespace-pre-wrap break-words text-sm leading-relaxed text-content-primary",
				className,
			)}
			{...props}
		/>
	);
};
