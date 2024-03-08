import { type CSSObject, useTheme } from "@emotion/react";
import Button from "@mui/material/Button";
import type { FC, ReactNode, FormEventHandler } from "react";

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
        borderRadius: 8,
        border: `1px solid ${theme.palette.divider}`,
        marginTop: 32,
      }}
      onSubmit={onSubmit}
    >
      <header css={{ padding: 24 }}>
        <div
          css={{
            fontSize: 20,
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
              marginTop: 8,
            }}
          >
            {subtitle}
          </div>
        )}
        <div css={[theme.typography.body2 as CSSObject, { paddingTop: 16 }]}>
          {children}
        </div>
      </header>
      <footer
        css={[
          theme.typography.body2 as CSSObject,
          {
            background: theme.palette.background.paper,
            padding: "16px 24px",
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
