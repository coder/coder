import { Separator as SeparatorPrimitive } from "@base-ui/react/separator";
import { cn } from "cn";

function Separator({
	className,
	orientation = "horizontal",
	decorative = true,
	...props
}: Omit<React.ComponentProps<typeof SeparatorPrimitive>, "className"> & {
	className?: string;
	decorative?: boolean;
}) {
	return (
		<SeparatorPrimitive
			data-slot="separator"
			role={decorative ? "none" : "separator"}
			aria-orientation={decorative ? undefined : orientation}
			orientation={orientation}
			className={cn(
				"bg-border shrink-0 data-[orientation=horizontal]:h-px data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-px",
				className,
			)}
			{...props}
		/>
	);
}

export { Separator };
