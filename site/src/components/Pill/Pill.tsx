import { PaletteColor, Theme } from "@mui/material/styles";
import { makeStyles } from "@mui/styles";
import { FC } from "react";
import { PaletteIndex } from "theme/theme";
import { combineClasses } from "utils/combineClasses";

export interface PillProps {
  className?: string;
  icon?: React.ReactNode;
  text: string;
  type?: PaletteIndex;
  lightBorder?: boolean;
  title?: string;
}

export const Pill: FC<PillProps> = (props) => {
  const { className, icon, text = false, title } = props;
  const styles = useStyles(props);
  return (
    <div
      className={combineClasses([styles.wrapper, styles.pillColor, className])}
      role="status"
      title={title}
    >
      {icon && <div className={styles.iconWrapper}>{icon}</div>}
      {text}
    </div>
  );
};

const useStyles = makeStyles<Theme, PillProps>((theme) => ({
  wrapper: {
    display: "inline-flex",
    alignItems: "center",
    borderWidth: 1,
    borderStyle: "solid",
    borderRadius: 99999,
    fontSize: 12,
    color: "#FFF",
    height: theme.spacing(3),
    paddingLeft: ({ icon }) =>
      icon ? theme.spacing(0.75) : theme.spacing(1.5),
    paddingRight: theme.spacing(1.5),
    whiteSpace: "nowrap",
    fontWeight: 400,
  },

  pillColor: {
    backgroundColor: ({ type }) =>
      type
        ? (theme.palette[type] as PaletteColor).dark
        : theme.palette.text.secondary,
    borderColor: ({ type, lightBorder }) =>
      type
        ? lightBorder
          ? (theme.palette[type] as PaletteColor).light
          : (theme.palette[type] as PaletteColor).main
        : theme.palette.text.secondary,
  },

  iconWrapper: {
    marginRight: theme.spacing(0.5),
    width: theme.spacing(1.75),
    height: theme.spacing(1.75),
    lineHeight: 0,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",

    "& > svg": {
      width: theme.spacing(1.75),
      height: theme.spacing(1.75),
    },
  },
}));
