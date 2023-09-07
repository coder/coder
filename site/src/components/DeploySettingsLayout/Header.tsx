import Button from "@mui/material/Button";
import { makeStyles } from "@mui/styles";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import { Stack } from "components/Stack/Stack";
import { FC } from "react";

export const Header: FC<{
  title: string | JSX.Element;
  description?: string | JSX.Element;
  secondary?: boolean;
  docsHref?: string;
}> = ({ title, description, docsHref, secondary }) => {
  const styles = useStyles();

  return (
    <Stack alignItems="baseline" direction="row" justifyContent="space-between">
      <div className={styles.headingGroup}>
        <h1 className={`${styles.title} ${secondary ? "secondary" : ""}`}>
          {title}
        </h1>
        {description && (
          <span className={styles.description}>{description}</span>
        )}
      </div>

      {docsHref && (
        <Button
          startIcon={<LaunchOutlined />}
          component="a"
          href={docsHref}
          target="_blank"
        >
          Read the docs
        </Button>
      )}
    </Stack>
  );
};

const useStyles = makeStyles((theme) => ({
  headingGroup: {
    maxWidth: 420,
    marginBottom: theme.spacing(3),
  },

  title: {
    fontSize: 32,
    fontWeight: 700,
    display: "flex",
    alignItems: "center",
    lineHeight: "initial",
    margin: 0,
    marginBottom: theme.spacing(0.5),
    gap: theme.spacing(1),

    "&.secondary": {
      fontSize: 24,
      fontWeight: 500,
    },
  },

  description: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
  },
}));
