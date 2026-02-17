import { forwardRef } from "react";
import { cn } from "utils/cn";

type ConversationProps = React.HTMLAttributes<HTMLDivElement>;

export const Conversation = forwardRef<HTMLDivElement, ConversationProps>(
	({ className, ...props }, ref) => {
		return (
			<div
				ref={ref}
				className={cn("flex flex-col gap-5", className)}
				{...props}
			/>
		);
	},
);

Conversation.displayName = "Conversation";

type ConversationItemProps = React.HTMLAttributes<HTMLDivElement> & {
	role: "user" | "assistant";
};

export const ConversationItem = forwardRef<HTMLDivElement, ConversationItemProps>(
	({ className, role, ...props }, ref) => {
		return (
			<div
				ref={ref}
				data-role={role}
				className={cn(
					"group flex w-full items-start gap-3",
					role === "user" && "justify-end",
					className,
				)}
				{...props}
			/>
		);
	},
);

ConversationItem.displayName = "ConversationItem";
