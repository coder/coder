/**
 * Copied from shadcn/ui and modified on 12/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/popover}
 */
import { Popover as PopoverPrimitive } from "radix-ui";
import { popperContentAnimationClass } from "#/components/Popper/popperClasses";
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
				max-h-(--radix-popper-available-height) overflow-y-auto`,
				popperContentAnimationClass,
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
