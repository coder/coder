import { type FC, type ReactNode } from "react";
import {
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import InfoIcon from "@mui/icons-material/InfoOutlined";
import { css } from "@emotion/css";
import { dark } from "theme/theme";

interface InfoTooltipProps {
  // TODO: use a `ThemeRole` type or something
  type?: "warning" | "notice" | "info";
  title: ReactNode;
  message: ReactNode;
}

const iconColor = {
  warning: dark.roles.warning.outline,
  notice: dark.roles.notice.outline,
  info: dark.roles.info.outline,
};

export const InfoTooltip: FC<InfoTooltipProps> = (props) => {
  const { title, message, type = "info" } = props;

  return (
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconClassName={css`
        color: ${iconColor[type]};
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
