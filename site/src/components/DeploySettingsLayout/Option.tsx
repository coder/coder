import { makeStyles } from "@mui/styles";
import { PropsWithChildren, FC } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { DisabledBadge, EnabledBadge } from "./Badges";
import Box, { BoxProps } from "@mui/material/Box";

export const OptionName: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles();
  return <span className={styles.optionName}>{children}</span>;
};

export const OptionDescription: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles();
  return <span className={styles.optionDescription}>{children}</span>;
};

const NotSet: FC = () => {
  const styles = useStyles();

  return <span className={styles.optionValue}>Not set</span>;
};

export const OptionValue: FC<{ children?: unknown }> = ({ children }) => {
  const styles = useStyles();

  if (typeof children === "boolean") {
    return children ? <EnabledBadge /> : <DisabledBadge />;
  }

  if (typeof children === "number") {
    return <span className={styles.optionValue}>{children}</span>;
  }

  if (typeof children === "string") {
    return <span className={styles.optionValue}>{children}</span>;
  }

  if (Array.isArray(children)) {
    if (children.length === 0) {
      return <NotSet />;
    }

    return (
      <ul className={styles.optionValueList}>
        {children.map((item) => (
          <li key={item} className={styles.optionValue}>
            {item}
          </li>
        ))}
      </ul>
    );
  }

  if (children === "") {
    return <NotSet />;
  }

  return <span className={styles.optionValue}>{JSON.stringify(children)}</span>;
};

export const OptionConfig = (props: BoxProps) => {
  return (
    <Box
      {...props}
      sx={{
        fontSize: 13,
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontWeight: 600,
        backgroundColor: (theme) => theme.palette.background.paperLight,
        display: "inline-flex",
        alignItems: "center",
        borderRadius: 0.25,
        padding: (theme) => theme.spacing(0, 1),
        border: (theme) => `1px solid ${theme.palette.divider}`,
        ...props.sx,
      }}
    />
  );
};

export const OptionConfigFlag = (props: BoxProps) => {
  return (
    <Box
      {...props}
      sx={{
        fontSize: 10,
        fontWeight: 600,
        margin: (theme) => theme.spacing(0, 0.75, 0, -0.5),
        display: "block",
        backgroundColor: (theme) => theme.palette.divider,
        lineHeight: 1,
        padding: (theme) => theme.spacing(0.25, 0.5),
        borderRadius: 0.25,
        ...props.sx,
      }}
    />
  );
};

const useStyles = makeStyles((theme) => ({
  optionName: {
    display: "block",
  },

  optionDescription: {
    display: "block",
    color: theme.palette.text.secondary,
    fontSize: 14,
    marginTop: theme.spacing(0.5),
  },

  optionValue: {
    fontSize: 14,
    fontFamily: MONOSPACE_FONT_FAMILY,
    overflowWrap: "anywhere",
    userSelect: "all",

    "& ul": {
      padding: theme.spacing(2),
    },
  },

  optionValueList: {
    margin: 0,
    padding: 0,
    listStylePosition: "inside",
    display: "flex",
    flexDirection: "column",
    gap: theme.spacing(0.5),
  },
}));
