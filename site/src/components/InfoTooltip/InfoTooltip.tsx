import { type FC, type ReactNode } from "react";
import {
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import InfoIcon from "@mui/icons-material/InfoOutlined";
import { css } from "@emotion/css";
import { colors } from "theme/colors";

interface InfoTooltipProps {
  type?: "warning" | "info";
  title: ReactNode;
  message: ReactNode;
}

export const InfoTooltip: FC<InfoTooltipProps> = (props) => {
  const { title, message, type = "info" } = props;

  return (
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconClassName={css`
        color: ${type === "info" ? colors.blue[5] : colors.yellow[5]};
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
