import type { ComponentPropsWithRef } from "react";
import { cn } from "utils/cn";

type ConversationProps = ComponentPropsWithRef<"div">;

export const Conversation = ({
	className,
	ref,
	...props
}: ConversationProps) => {
	return (
		<div
			ref={ref}
			className={cn("flex flex-col gap-5", className)}
			{...props}
		/>
	);
};

type ConversationItemProps = Omit<ComponentPropsWithRef<"div">, "role"> & {
	role: "user" | "assistant";
};

export const ConversationItem = ({
	className,
	role,
	ref,
	...props
}: ConversationItemProps) => {
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
};
