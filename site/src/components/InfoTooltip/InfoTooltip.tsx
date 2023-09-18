import { type FC, type ReactNode } from "react";
import {
  HelpTooltip,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import InfoIcon from "@mui/icons-material/InfoOutlined";
import { makeStyles } from "@mui/styles";
import { colors } from "theme/colors";

interface InfoTooltipProps {
  type?: "warning" | "info";
  title: ReactNode;
  message: ReactNode;
}

export const InfoTooltip: FC<InfoTooltipProps> = (props) => {
  const { title, message, type = "info" } = props;

  const styles = useStyles({ type });

  return (
    <HelpTooltip
      size="small"
      icon={InfoIcon}
      iconClassName={styles.icon}
      buttonClassName={styles.button}
    >
      <HelpTooltipTitle>{title}</HelpTooltipTitle>
      <HelpTooltipText>{message}</HelpTooltipText>
    </HelpTooltip>
  );
};

const useStyles = makeStyles<unknown, Pick<InfoTooltipProps, "type">>(() => ({
  icon: ({ type }) => ({
    color: type === "info" ? colors.blue[5] : colors.yellow[5],
  }),

  button: {
    opacity: 1,

    "&:hover": {
      opacity: 1,
    },
  },
}));
