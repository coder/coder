import { type FC, type ReactNode } from "react";
import {
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import InfoIcon from "@mui/icons-material/InfoOutlined";
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
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconStyles={{ color: theme.experimental.roles[type].outline }}
      buttonStyles={styles.button}
    >
      <HelpTooltipTitle>{title}</HelpTooltipTitle>
      <HelpTooltipText>{message}</HelpTooltipText>
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
