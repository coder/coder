import { useTheme } from "@emotion/react";
import type { FC } from "react";
import type { ThemeRole } from "theme/roles";

interface StatusIndicatorProps {
	color: ThemeRole;
	variant?: "solid" | "outlined";
}

export const StatusIndicator: FC<StatusIndicatorProps> = ({
	color,
	variant = "solid",
}) => {
	const theme = useTheme();

	return (
		<div
			css={[
				{
					height: 8,
					width: 8,
					borderRadius: 4,
				},
				variant === "solid" && {
					backgroundColor: theme.roles[color].fill.solid,
				},
				variant === "outlined" && {
					border: `1px solid ${theme.roles[color].outline}`,
				},
			]}
		/>
	);
};
