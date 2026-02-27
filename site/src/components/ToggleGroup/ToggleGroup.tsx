/**
 * Copied from shadcn/ui on 02/27/2026
 * @see {@link https://ui.shadcn.com/docs/components/toggle-group}
 */

import * as ToggleGroupPrimitive from "@radix-ui/react-toggle-group";
import type { VariantProps } from "class-variance-authority";
import { toggleVariants } from "components/Toggle/Toggle";
import * as React from "react";
import { cn } from "utils/cn";

const ToggleGroupContext = React.createContext<
	VariantProps<typeof toggleVariants> & {
		spacing?: number;
		orientation?: "horizontal" | "vertical";
	}
>({
	size: "default",
	variant: "default",
	spacing: 0,
	orientation: "horizontal",
});

type ToggleGroupProps = React.ComponentProps<typeof ToggleGroupPrimitive.Root> &
	VariantProps<typeof toggleVariants> & {
		spacing?: number;
		orientation?: "horizontal" | "vertical";
	};

export const ToggleGroup: React.FC<ToggleGroupProps> = ({
	className,
	variant,
	size,
	spacing = 0,
	orientation = "horizontal",
	children,
	...props
}) => {
	return (
		<ToggleGroupPrimitive.Root
			style={{ gap: spacing > 0 ? `${spacing}px` : undefined }}
			className={cn(
				"inline-flex w-fit items-center rounded-md",
				orientation === "vertical" ? "flex-col items-stretch" : "flex-row",
				spacing === 0 ? "gap-0" : "",
				className,
			)}
			{...props}
		>
			<ToggleGroupContext.Provider
				value={{ variant, size, spacing, orientation }}
			>
				{children}
			</ToggleGroupContext.Provider>
		</ToggleGroupPrimitive.Root>
	);
};

type ToggleGroupItemProps = React.ComponentProps<
	typeof ToggleGroupPrimitive.Item
> &
	VariantProps<typeof toggleVariants>;

export const ToggleGroupItem: React.FC<ToggleGroupItemProps> = ({
	className,
	children,
	variant = "default",
	size = "default",
	...props
}) => {
	const context = React.useContext(ToggleGroupContext);
	const itemVariant = context.variant || variant;
	const itemSize = context.size || size;
	const hasSpacing = (context.spacing || 0) > 0;
	const isVertical = context.orientation === "vertical";

	return (
		<ToggleGroupPrimitive.Item
			className={cn(
				toggleVariants({ variant: itemVariant, size: itemSize }),
				"shrink-0 focus:z-10 focus-visible:z-10",
				!hasSpacing &&
					(isVertical
						? "rounded-none first:rounded-t-md last:rounded-b-md"
						: "rounded-none first:rounded-l-md last:rounded-r-md"),
				!hasSpacing && itemVariant === "outline"
					? "[&:not(:first-child)]:-ml-px"
					: "",
				className,
			)}
			{...props}
		>
			{children}
		</ToggleGroupPrimitive.Item>
	);
};
