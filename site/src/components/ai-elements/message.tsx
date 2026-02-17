import { forwardRef } from "react";
import { cn } from "utils/cn";

type MessageProps = React.HTMLAttributes<HTMLDivElement>;

export const Message = forwardRef<HTMLDivElement, MessageProps>(
	({ className, ...props }, ref) => {
		return (
			<div
				ref={ref}
				className={cn("max-w-full min-w-0", className)}
				{...props}
			/>
		);
	},
);

Message.displayName = "Message";

type MessageAvatarProps = React.HTMLAttributes<HTMLDivElement>;

export const MessageAvatar = forwardRef<HTMLDivElement, MessageAvatarProps>(
	({ className, ...props }, ref) => {
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
	},
);

MessageAvatar.displayName = "MessageAvatar";

type MessageHeaderProps = React.HTMLAttributes<HTMLDivElement>;

export const MessageHeader = forwardRef<HTMLDivElement, MessageHeaderProps>(
	({ className, ...props }, ref) => {
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
	},
);

MessageHeader.displayName = "MessageHeader";

type MessageContentProps = React.HTMLAttributes<HTMLDivElement>;

export const MessageContent = forwardRef<HTMLDivElement, MessageContentProps>(
	({ className, ...props }, ref) => {
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
	},
);

MessageContent.displayName = "MessageContent";
