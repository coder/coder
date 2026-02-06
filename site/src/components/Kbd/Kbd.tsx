import { cn } from "utils/cn";

function Kbd({ className, ...props }: React.ComponentProps<"kbd">) {
	return (
		<kbd
			data-slot="kbd"
			className={cn(
				"inline-flex items-center justify-center gap-1",
				"h-5 w-fit min-w-5 font-sans text-xs font-medium select-none5",
				"bg-surface-tertiary text-content-secondary rounded-sm px-1",
				"[&_svg:not([class*='size-'])]:size-3 pointer-events-none",
				className,
			)}
			{...props}
		/>
	);
}
function KbdGroup({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<kbd
			data-slot="kbd-group"
			className={cn("gap-1 inline-flex items-center", className)}
			{...props}
		/>
	);
}
export { Kbd, KbdGroup };
