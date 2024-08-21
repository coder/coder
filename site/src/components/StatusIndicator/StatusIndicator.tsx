import { useTheme } from "@emotion/react";
import type { FC } from "react";
import type { ThemeRole } from "theme/colorRoles";

interface StatusIndicatorProps {
	color: ThemeRole;
}

export const StatusIndicator: FC<StatusIndicatorProps> = ({ color }) => {
	const theme = useTheme();

	return (
		<div
			css={{
				height: 8,
				width: 8,
				borderRadius: 4,
				backgroundColor: theme.colorRoles[color].fill.solid,
			}}
		/>
	);
};
