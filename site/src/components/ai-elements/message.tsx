import type { ComponentPropsWithRef } from "react";
import { cn } from "utils/cn";

type MessageProps = ComponentPropsWithRef<"div">;

export const Message = ({ className, ref, ...props }: MessageProps) => {
	return (
		<div ref={ref} className={cn("max-w-full min-w-0", className)} {...props} />
	);
};

type MessageAvatarProps = ComponentPropsWithRef<"div">;

const MessageAvatar = ({
	className,
	ref,
	...props
}: MessageAvatarProps) => {
	return (
		<div
			ref={ref}
			className={cn(
				"mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-border-default/70 shadow-sm",
				className,
			)}
			{...props}
		/>
	);
};

type MessageHeaderProps = ComponentPropsWithRef<"div">;

const MessageHeader = ({
	className,
	ref,
	...props
}: MessageHeaderProps) => {
	return (
		<div
			ref={ref}
			className={cn(
				"mb-1 text-xs font-medium text-content-secondary",
				className,
			)}
			{...props}
		/>
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
