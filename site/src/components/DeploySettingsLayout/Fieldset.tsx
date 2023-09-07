import { makeStyles } from "@mui/styles";
import { FC, ReactNode, FormEventHandler } from "react";
import Button from "@mui/material/Button";

export const Fieldset: FC<{
  children: ReactNode;
  title: string | JSX.Element;
  subtitle?: string | JSX.Element;
  validation?: string | JSX.Element | false;
  button?: JSX.Element | false;
  onSubmit: FormEventHandler<HTMLFormElement>;
  isSubmitting?: boolean;
}> = ({
  title,
  subtitle,
  children,
  validation,
  button,
  onSubmit,
  isSubmitting,
}) => {
  const styles = useStyles();

  return (
    <form className={styles.fieldset} onSubmit={onSubmit}>
      <header className={styles.header}>
        <div className={styles.title}>{title}</div>
        {subtitle && <div className={styles.subtitle}>{subtitle}</div>}
        <div className={styles.body}>{children}</div>
      </header>
      <footer className={styles.footer}>
        <div className={styles.validation}>{validation}</div>
        {button || (
          <Button type="submit" disabled={isSubmitting}>
            Submit
          </Button>
        )}
      </footer>
    </form>
  );
};

const useStyles = makeStyles((theme) => ({
  fieldset: {
    borderRadius: theme.spacing(1),
    border: `1px solid ${theme.palette.divider}`,
    background: theme.palette.background.paper,
    marginTop: theme.spacing(4),
  },
  title: {
    fontSize: theme.spacing(2.5),
    margin: 0,
    fontWeight: 600,
  },
  subtitle: {
    color: theme.palette.text.secondary,
    fontSize: 14,
    marginTop: theme.spacing(1),
  },
  body: {
    ...theme.typography.body2,
    paddingTop: theme.spacing(2),
  },
  validation: {
    color: theme.palette.text.secondary,
  },
  header: {
    padding: theme.spacing(3),
  },
  footer: {
    ...theme.typography.body2,
    background: theme.palette.background.paperLight,
    padding: `${theme.spacing(2)} ${theme.spacing(3)}`,
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
  },
}));
