import { Popover as PopoverPrimitive } from "radix-ui";
import { cn } from "#/utils/cn";

export type PopoverContentProps = React.ComponentPropsWithRef<
	typeof PopoverPrimitive.Content
> & {
	disablePortal?: boolean;
};

export const Popover = PopoverPrimitive.Root;

export const PopoverTrigger = PopoverPrimitive.Trigger;

export const PopoverAnchor = PopoverPrimitive.Anchor;

export const PopoverContent: React.FC<PopoverContentProps> = ({
	className,
	align = "center",
	sideOffset = 4,
	disablePortal,
	...props
}) => {
	const content = (
		<PopoverPrimitive.Content
			align={align}
			sideOffset={sideOffset}
			collisionPadding={16}
			className={cn(
				`z-50 w-72 rounded-md border border-solid bg-surface-primary
				text-content-primary shadow-md outline-hidden
				max-h-(--radix-popper-available-height) overflow-y-auto
				data-[state=open]:animate-in data-[state=closed]:animate-out
				data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0
				data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95
				data-[state=open]:data-[side=bottom]:slide-in-from-top-2 data-[state=open]:data-[side=left]:slide-in-from-right-2
				data-[state=open]:data-[side=right]:slide-in-from-left-2 data-[state=open]:data-[side=top]:slide-in-from-bottom-2`,
				className,
			)}
			{...props}
		/>
	);

	return disablePortal ? (
		content
	) : (
		<PopoverPrimitive.Portal>{content}</PopoverPrimitive.Portal>
	);
};
