import { type FC, type ReactNode } from "react";
import {
  HelpTooltip,
  HelpTooltipContent,
  HelpTooltipIcon,
  HelpTooltipText,
  HelpTooltipTitle,
  HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import type { ThemeRole } from "theme/experimental";

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
  const iconColor = theme.experimental.roles[type].outline;

  return (
    <HelpTooltip>
      <HelpTooltipTrigger size="small" css={styles.button}>
        <HelpTooltipIcon css={{ color: iconColor }} />
      </HelpTooltipTrigger>
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
