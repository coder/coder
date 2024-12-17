import * as AvatarPrimitive from "@radix-ui/react-avatar";
import { type VariantProps, cva } from "class-variance-authority";
/**
 * Copied from shadc/ui on 12/16/2024
 * @see {@link https://ui.shadcn.com/docs/components/avatar}
 *
 * This component was updated to support the variants and match the styles from
 * the Figma design:
 * @see {@link https://www.figma.com/design/WfqIgsTFXN2BscBSSyXWF8/Coder-kit?node-id=711-383&t=xqxOSUk48GvDsjGK-0}
 */
import * as React from "react";
import { cn } from "utils/cn";

const avatarVariants = cva(
	"relative flex shrink-0 overflow-hidden rounded border border-solid bg-surface-secondary text-content-secondary",
	{
		variants: {
			size: {
				lg: "h-10 w-10 rounded-[6px] text-sm font-medium",
				default: "h-6 w-6 text-2xs",
				sm: "h-[18px] w-[18px] text-[8px]",
			},
			variant: {
				default: "",
				icon: "",
			},
		},
		defaultVariants: {
			size: "default",
		},
		compoundVariants: [
			{
				size: "lg",
				variant: "icon",
				className: "p-[9px]",
			},
			{
				size: "default",
				variant: "icon",
				className: "p-[3px]",
			},
			{
				size: "sm",
				variant: "icon",
				className: "p-[2px]",
			},
		],
	},
);

export interface AvatarProps
	extends React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Root>,
		VariantProps<typeof avatarVariants> {}

const Avatar = React.forwardRef<
	React.ElementRef<typeof AvatarPrimitive.Root>,
	AvatarProps
>(({ className, size, variant, ...props }, ref) => (
	<AvatarPrimitive.Root
		ref={ref}
		className={cn(avatarVariants({ size, variant, className }))}
		{...props}
	/>
));
Avatar.displayName = AvatarPrimitive.Root.displayName;

const AvatarImage = React.forwardRef<
	React.ElementRef<typeof AvatarPrimitive.Image>,
	React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Image>
>(({ className, ...props }, ref) => (
	<AvatarPrimitive.Image
		ref={ref}
		className={cn("aspect-square h-full w-full", className)}
		{...props}
	/>
));
AvatarImage.displayName = AvatarPrimitive.Image.displayName;

const AvatarFallback = React.forwardRef<
	React.ElementRef<typeof AvatarPrimitive.Fallback>,
	React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Fallback>
>(({ className, ...props }, ref) => (
	<AvatarPrimitive.Fallback
		ref={ref}
		className={cn(
			"flex h-full w-full items-center justify-center rounded-full",
			className,
		)}
		{...props}
	/>
));
AvatarFallback.displayName = AvatarPrimitive.Fallback.displayName;

export { Avatar, AvatarImage, AvatarFallback };
