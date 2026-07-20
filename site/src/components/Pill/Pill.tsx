import { cva } from "class-variance-authority";
import type { FC, ReactNode } from "react";
import type { ThemeRole } from "#/theme/roles";
import { cn } from "#/utils/cn";
import { Spinner } from "../Spinner/Spinner";

type PillType = ThemeRole | "muted";

type PillProps = React.ComponentPropsWithRef<"div"> & {
	icon?: ReactNode;
	type?: PillType;
	size?: "md" | "lg";
};

const pillRoleVariants = cva("text-content-primary", {
	variants: {
		type: {
			error: "border-border-destructive bg-surface-red",
			warning: "border-border-warning bg-surface-orange",
			notice: "border-border-pending bg-surface-sky",
			info: "border-border bg-surface-secondary",
			success: "border-border-success bg-surface-green",
			active: "border-border-pending bg-surface-sky",
			inactive: "border-border bg-surface-secondary",
			danger: "border-border-warning bg-surface-orange",
			preview: "border-border-purple bg-surface-purple",
			muted:
				"border-border-secondary bg-surface-tertiary text-content-secondary",
		},
	},
	defaultVariants: {
		type: "inactive",
	},
});

const pillLayoutVariants = cva(
	"inline-flex cursor-default items-center whitespace-nowrap rounded-full border border-solid text-[12px] font-normal leading-none [&_svg]:size-[14px]",
	{
		variants: {
			size: {
				md: "h-6",
				lg: "",
			},
			withIcon: {
				true: "",
				false: "",
			},
		},
		compoundVariants: [
			{
				size: "md",
				withIcon: false,
				class: "gap-[5px] px-3",
			},
			{
				size: "md",
				withIcon: true,
				class: "gap-[5px] pr-3 pl-[5px]",
			},
			{
				size: "lg",
				withIcon: false,
				class: "gap-2.5 py-3.5 px-4",
			},
			{
				size: "lg",
				withIcon: true,
				class: "gap-2.5 py-3.5 pr-4 pl-2.5",
			},
		],
		defaultVariants: {
			size: "md",
			withIcon: false,
		},
	},
);

export const PillSpinner: FC = () => {
	return <Spinner loading />;
};

export const Pill: FC<PillProps> = ({
	icon,
	type = "inactive",
	children,
	size = "md",
	className,
	...divProps
}) => {
	return (
		<div
			className={cn(
				pillLayoutVariants({ size, withIcon: Boolean(icon) }),
				pillRoleVariants({ type }),
				className,
			)}
			{...divProps}
		>
			{icon}
			{children}
		</div>
	);
};
