import { cva } from "class-variance-authority";
import type { FC, ReactNode } from "react";
import type { ThemeRole } from "#/theme/roles";
import { cn } from "#/utils/cn";
import { Spinner, type SpinnerProps } from "../Spinner/Spinner";

type PillProps = React.ComponentPropsWithRef<"div"> & {
	icon?: ReactNode;
	type?: ThemeRole;
	size?: "md" | "lg";
};

const pillRoleVariants = cva("text-content-primary", {
	variants: {
		type: {
			error: "border-border-destructive bg-surface-red",
			warning: "border-border-warning bg-surface-orange",
			notice: "border-border-pending bg-surface-sky",
			info: "border-border bg-surface-quaternary",
			success: "border-border-green bg-surface-green",
			active: "border-border-pending bg-surface-sky",
			inactive: "border-border bg-surface-secondary",
			danger: "border-border-warning bg-surface-orange",
			preview: "border-border-purple bg-surface-purple",
		},
	},
	defaultVariants: {
		type: "inactive",
	},
});

const pillLayoutVariants = cva(
	"inline-flex cursor-default items-center whitespace-nowrap border border-solid font-normal [&_svg]:size-[14px]",
	{
		variants: {
			size: {
				md: "h-6 rounded-full text-xs leading-none",
				lg: "rounded-full py-3.5 pl-4 pr-4 text-sm leading-none",
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
				class: "gap-2.5",
			},
			{
				size: "lg",
				withIcon: true,
				class: "gap-2.5 pl-2.5 pr-4",
			},
		],
		defaultVariants: {
			size: "md",
			withIcon: false,
		},
	},
);

type PillSpinnerProps = SpinnerProps;

export const PillSpinner: FC<PillSpinnerProps> = ({ size = "sm" }) => {
	return <Spinner size={size} loading />;
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
