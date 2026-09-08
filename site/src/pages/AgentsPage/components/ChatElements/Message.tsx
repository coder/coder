import { cn } from "cn";
import type { ComponentPropsWithRef } from "react";

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
				"whitespace-pre-wrap wrap-break-word text-[13px] leading-relaxed text-content-primary",
				className,
			)}
			{...props}
		/>
	);
};
