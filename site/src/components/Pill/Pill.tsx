import { useTheme } from "@emotion/react";
import CircularProgress, {
	type CircularProgressProps,
} from "@mui/material/CircularProgress";
import { type FC, type ReactNode, useMemo } from "react";
import type { ThemeRole } from "#/theme/roles";
import { cn } from "#/utils/cn";

type PillProps = React.ComponentPropsWithRef<"div"> & {
	icon?: ReactNode;
	type?: ThemeRole;
	size?: "md" | "lg";
};

const PILL_ICON_SIZE = 14;

export const Pill: FC<PillProps> = ({
	icon,
	type = "inactive",
	children,
	size = "md",
	className,
	style,
	...divProps
}) => {
	const theme = useTheme();
	const fillStyles = useMemo(() => {
		const palette = theme.roles[type];
		return {
			backgroundColor: palette.background,
			borderColor: palette.outline,
			color: palette.text,
		};
	}, [theme, type]);

	return (
		<div
			className={cn(
				"inline-flex items-center whitespace-nowrap rounded-full border border-solid",
				"font-normal text-xs leading-none cursor-default",
				"[&>svg]:size-[14px]",
				size === "md" && "h-6 gap-[5px] px-3",
				Boolean(icon) && size === "md" && "pl-[5px]",
				size === "lg" && "gap-[10px] px-4 py-2",
				Boolean(icon) && size === "lg" && "pl-[10px]",
				className,
			)}
			style={{ ...fillStyles, ...style }}
			{...divProps}
		>
			{icon}
			{children}
		</div>
	);
};

export const PillSpinner: FC<CircularProgressProps> = (props) => {
	const theme = useTheme();
	return (
		<CircularProgress
			size={PILL_ICON_SIZE}
			sx={{ "& svg": { transform: "scale(.75)" } }}
			style={{ color: theme.experimental.l1.text }}
			{...props}
		/>
	);
};
