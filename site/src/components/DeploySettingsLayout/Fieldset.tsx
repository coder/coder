import type { FC, ReactNode, FormEventHandler } from "react";
import Button from "@mui/material/Button";
import { type CSSObject, useTheme } from "@emotion/react";

interface FieldsetProps {
  children: ReactNode;
  title: string | JSX.Element;
  subtitle?: string | JSX.Element;
  validation?: string | JSX.Element | false;
  button?: JSX.Element | false;
  onSubmit: FormEventHandler<HTMLFormElement>;
  isSubmitting?: boolean;
}

export const Fieldset: FC<FieldsetProps> = (props) => {
  const {
    title,
    subtitle,
    children,
    validation,
    button,
    onSubmit,
    isSubmitting,
  } = props;
  const theme = useTheme();

  return (
    <form
      css={{
        borderRadius: theme.spacing(1),
        border: `1px solid ${theme.palette.divider}`,
        background: theme.palette.background.paper,
        marginTop: theme.spacing(4),
      }}
      onSubmit={onSubmit}
    >
      <header css={{ padding: theme.spacing(3) }}>
        <div
          css={{
            fontSize: theme.spacing(2.5),
            margin: 0,
            fontWeight: 600,
          }}
        >
          {title}
        </div>
        {subtitle && (
          <div
            css={{
              color: theme.palette.text.secondary,
              fontSize: 14,
              marginTop: theme.spacing(1),
            }}
          >
            {subtitle}
          </div>
        )}
        <div
          css={[
            theme.typography.body2 as CSSObject,
            { paddingTop: theme.spacing(2) },
          ]}
        >
          {children}
        </div>
      </header>
      <footer
        css={[
          theme.typography.body2 as CSSObject,
          {
            background: theme.palette.background.paperLight,
            padding: `${theme.spacing(2)} ${theme.spacing(3)}`,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
          },
        ]}
      >
        <div css={{ color: theme.palette.text.secondary }}>{validation}</div>
        {button || (
          <Button type="submit" disabled={isSubmitting}>
            Submit
          </Button>
        )}
      </footer>
    </form>
  );
};
