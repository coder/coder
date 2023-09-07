import Box from "@mui/material/Box";
import { makeStyles } from "@mui/styles";
import Typography from "@mui/material/Typography";
import { FC, ReactNode } from "react";
import { combineClasses } from "../../utils/combineClasses";

export interface EmptyStateProps {
  /** Text Message to display, placed inside Typography component */
  message: string;
  /** Longer optional description to display below the message */
  description?: string | React.ReactNode;
  cta?: ReactNode;
  className?: string;
  image?: ReactNode;
}

/**
 * Component to place on screens or in lists that have no content. Optionally
 * provide a button that would allow the user to return from where they were,
 * or to add an item that they currently have none of.
 *
 * EmptyState's props extend the [Material UI Box component](https://material-ui.com/components/box/)
 * that you can directly pass props through to to customize the shape and layout of it.
 */
export const EmptyState: FC<React.PropsWithChildren<EmptyStateProps>> = (
  props,
) => {
  const { message, description, cta, className, image, ...boxProps } = props;
  const styles = useStyles();

  return (
    <Box className={combineClasses([styles.root, className])} {...boxProps}>
      <Typography variant="h5" className={styles.title}>
        {message}
      </Typography>
      {description && (
        <Typography
          variant="body2"
          color="textSecondary"
          className={styles.description}
        >
          {description}
        </Typography>
      )}
      {cta && <div className={styles.cta}>{cta}</div>}
      {image}
    </Box>
  );
};

const useStyles = makeStyles((theme) => ({
  root: {
    overflow: "hidden",
    display: "flex",
    flexDirection: "column",
    justifyContent: "center",
    alignItems: "center",
    textAlign: "center",
    minHeight: 360,
    padding: theme.spacing(10, 5),
    position: "relative",
  },

  title: {
    fontSize: theme.spacing(3),
  },

  description: {
    marginTop: theme.spacing(1.5),
    fontSize: theme.spacing(2),
    lineHeight: "140%",
    maxWidth: theme.spacing(60),
  },

  cta: {
    marginTop: theme.spacing(4),
  },
}));
