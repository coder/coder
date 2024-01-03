import { type FC, type ReactNode } from "react";
import {
  HelpTooltip,
  HelpTooltipContent,
  HelpTooltipIcon,
  HelpTooltipText,
  HelpTooltipTitle,
  HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Interpolation, Theme, css, useTheme } from "@emotion/react";

interface InfoTooltipProps {
  // TODO: use a `ThemeRole` type or something
  type?: "warning" | "notice" | "info";
  title: ReactNode;
  message: ReactNode;
}

export const InfoTooltip: FC<InfoTooltipProps> = ({
  title,
  message,
  type = "info",
}) => {
  const theme = useTheme();

  return (
    <HelpTooltip>
      <HelpTooltipTrigger size="small" css={styles.button}>
        <HelpTooltipIcon
          css={{ color: theme.experimental.roles[type].outline }}
        />
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
