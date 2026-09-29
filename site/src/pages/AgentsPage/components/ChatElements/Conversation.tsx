import { cn } from "cn";

type ConversationProps = React.ComponentProps<"div">;

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

type ConversationItemProps = Omit<React.ComponentProps<"div">, "role"> & {
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
