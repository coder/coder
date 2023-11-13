import { type FC, type ReactNode } from "react";
import {
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import InfoIcon from "@mui/icons-material/InfoOutlined";
import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";

interface InfoTooltipProps {
  // TODO: use a `ThemeRole` type or something
  type?: "warning" | "notice" | "info";
  title: ReactNode;
  message: ReactNode;
}

export const InfoTooltip: FC<InfoTooltipProps> = (props) => {
  const { title, message, type = "info" } = props;

  const theme = useTheme();
  const iconColor = theme.experimental.roles[type].outline;

  return (
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconClassName={css`
        color: ${iconColor};
      `}
      buttonClassName={css`
        opacity: 1;
        &:hover {
          opacity: 1;
        }
      `}
    >
      <HelpTooltipTitle>{title}</HelpTooltipTitle>
      <HelpTooltipText>{message}</HelpTooltipText>
    </HelpTooltip>
  );
};
