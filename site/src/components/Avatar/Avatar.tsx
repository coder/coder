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
import { cva, type VariantProps } from "class-variance-authority";
import { Avatar as AvatarPrimitive } from "radix-ui";
import { useAppearance } from "#/theme/appearance";
import { getExternalImageStylesFromUrl } from "#/theme/externalImages";
import { cn } from "#/utils/cn";

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
				emoji: null,
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

/**
 * Avatar props. The variant prop is resolved internally: built-in emoji
 * sources always render with the emoji variant, overriding any variant passed
 * by the caller, so emojis look the same at call sites whose source is
 * data-dependent and may be an icon, photo, or emoji.
 */
export type AvatarProps = AvatarPrimitive.AvatarProps &
	VariantProps<typeof avatarVariants> & {
		src?: string;
		fallback?: string;
		/**
		 * Alt text for the inner `<img>`. Defaults to `""` (decorative,
		 * hidden from assistive tech). Pass a descriptive value when no
		 * adjacent text identifies the content.
		 */
		alt?: string;
		ref?: React.Ref<React.ComponentRef<typeof AvatarPrimitive.Root>>;
	};

export const Avatar: React.FC<AvatarProps> = ({
	className,
	size,
	variant,
	src,
	fallback,
	alt = "",
	children,
	style,
	...props
}) => {
	const { externalImages } = useAppearance();

	// Built-in emojis always use the emoji variant, even when a caller
	// passes another one. See the AvatarProps doc.
	const resolvedVariant = src?.startsWith("/emojis/") ? "emoji" : variant;

	return (
		<AvatarPrimitive.Root
			className={cn(
				avatarVariants({ size, variant: resolvedVariant, className }),
			)}
			style={{
				...style,
				padding: resolvedVariant === "emoji" ? "20%" : style?.padding,
			}}
			{...props}
		>
			<AvatarPrimitive.Image
				src={src}
				alt={alt}
				className="aspect-square size-full object-contain"
				style={getExternalImageStylesFromUrl(externalImages, src)}
			/>
			{fallback && (
				<AvatarPrimitive.Fallback className="flex h-full w-full items-center justify-center rounded-full">
					{fallback.slice(0, 2).toUpperCase()}
				</AvatarPrimitive.Fallback>
			)}
			{children}
		</AvatarPrimitive.Root>
	);
};
