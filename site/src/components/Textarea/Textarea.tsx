import { cn } from "#/utils/cn";

export const Textarea: React.FC<React.ComponentPropsWithRef<"textarea">> = ({
	className,
	...props
}) => {
	return (
		<textarea
			className={cn(
				`flex min-h-[60px] w-full px-3 py-2 text-sm font-sans shadow-xs text-content-primary
				rounded-md border border-border bg-transparent placeholder:text-content-secondary
				focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link
				disabled:cursor-not-allowed disabled:opacity-50 disabled:text-content-disabled md:text-sm`,
				className,
			)}
			{...props}
		/>
	);
};
