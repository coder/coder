/**
 * Copied from shadc/ui on 12/16/2024
 * @see {@link https://ui.shadcn.com/docs/components/avatar}
 *
 * This component was updated to support the variants and match the styles from
 * the Figma design:
 * @see {@link https://www.figma.com/design/WfqIgsTFXN2BscBSSyXWF8/Coder-kit?node-id=711-383&t=xqxOSUk48GvDsjGK-0}
 *
 * It was also simplified to make usage easier and reduce boilerplate.
 * @see {@link https://github.com/coder/coder/pull/15930#issuecomment-2552292440}
 */

import { useTheme } from "@emotion/react";
import * as AvatarPrimitive from "@radix-ui/react-avatar";
import { type VariantProps, cva } from "class-variance-authority";
import * as React from "react";
import { getExternalImageStylesFromUrl } from "theme/externalImages";
import { cn } from "utils/cn";

const avatarVariants = cva(
	"relative flex shrink-0 overflow-hidden rounded border border-solid bg-surface-secondary text-content-secondary",
	{
		variants: {
			size: {
				lg: "size-[--avatar-lg] rounded-[6px] text-sm font-medium",
				md: "size-[--avatar-default] text-2xs",
				sm: "size-[--avatar-sm] text-[8px]",
			},
			variant: {
				default: null,
				icon: "[&_svg]:size-full",
			},
		},
		defaultVariants: {
			size: "md",
		},
		compoundVariants: [
			{
				size: "lg",
				variant: "icon",
				className: "p-2",
			},
			{
				size: "md",
				variant: "icon",
				className: "p-1",
			},
			{
				size: "sm",
				variant: "icon",
				className: "p-[3px]",
			},
		],
	},
);

export type AvatarProps = AvatarPrimitive.AvatarProps &
	VariantProps<typeof avatarVariants> & {
		src?: string;
		fallback?: string;
	};

const Avatar = React.forwardRef<
	React.ElementRef<typeof AvatarPrimitive.Root>,
	AvatarProps
>(({ className, size, variant, src, fallback, children, ...props }, ref) => {
	const theme = useTheme();

	return (
		<AvatarPrimitive.Root
			ref={ref}
			className={cn(avatarVariants({ size, variant, className }))}
			{...props}
		>
			<AvatarPrimitive.Image
				src={src}
				className="aspect-square h-full w-full object-contain"
				css={getExternalImageStylesFromUrl(theme.externalImages, src)}
			/>
			{fallback && (
				<AvatarPrimitive.Fallback className="flex h-full w-full items-center justify-center rounded-full">
					{fallback.charAt(0).toUpperCase()}
				</AvatarPrimitive.Fallback>
			)}
			{children}
		</AvatarPrimitive.Root>
	);
});
Avatar.displayName = AvatarPrimitive.Root.displayName;

export { Avatar };
