import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipIcon,
	HelpTooltipIconTrigger,
	HelpTooltipText,
	HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import type { FC, ReactNode } from "react";
import type { ThemeRole } from "theme/roles";

interface InfoTooltipProps {
	type?: ThemeRole;
	title: ReactNode;
	message: ReactNode;
}

export const InfoTooltip: FC<InfoTooltipProps> = ({
	title,
	message,
	type = "info",
}) => {
	const theme = useTheme();
	const iconColor = theme.roles[type].outline;

	return (
		<HelpTooltip>
			<HelpTooltipIconTrigger size="small" css={styles.button}>
				<HelpTooltipIcon css={{ color: iconColor }} />
			</HelpTooltipIconTrigger>
			<HelpTooltipContent>
				<HelpTooltipTitle>{title}</HelpTooltipTitle>
				<HelpTooltipText>{message}</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

const styles = {
	button: css`
		opacity: 1;

		&:hover {
			opacity: 1;
		}
	`,
} satisfies Record<string, Interpolation<Theme>>;
