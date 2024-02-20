import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import {
  createContext,
  type FC,
  type HTMLProps,
  useContext,
  ReactNode,
  ComponentProps,
} from "react";
import { AlphaBadge, DeprecatedBadge } from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";
import {
  FormFooter as BaseFormFooter,
  FormFooterProps,
  type FormFooterStyles,
} from "../FormFooter/FormFooter";

type FormContextValue = { direction?: "horizontal" | "vertical" };

const FormContext = createContext<FormContextValue>({
  direction: "horizontal",
});

type FormProps = HTMLProps<HTMLFormElement> & {
  direction?: FormContextValue["direction"];
};

export const Form: FC<FormProps> = ({ direction, ...formProps }) => {
  const theme = useTheme();

  return (
    <FormContext.Provider value={{ direction }}>
      <form
        {...formProps}
        css={{
          display: "flex",
          flexDirection: "column",
          gap: direction === "horizontal" ? 80 : 40,

          [theme.breakpoints.down("md")]: {
            gap: 64,
          },
        }}
      />
    </FormContext.Provider>
  );
};

export const HorizontalForm: FC<HTMLProps<HTMLFormElement>> = ({
  children,
  ...formProps
}) => {
  return (
    <Form direction="horizontal" {...formProps}>
      {children}
    </Form>
  );
};

export const VerticalForm: FC<HTMLProps<HTMLFormElement>> = ({
  children,
  ...formProps
}) => {
  return (
    <Form direction="vertical" {...formProps}>
      {children}
    </Form>
  );
};

interface FormSectionProps {
  children?: ReactNode;
  title: ReactNode;
  description: ReactNode;
  classes?: {
    root?: string;
    sectionInfo?: string;
    infoTitle?: string;
  };
  alpha?: boolean;
  deprecated?: boolean;
}

export const FormSection: FC<FormSectionProps> = ({
  children,
  title,
  description,
  classes = {},
  alpha = false,
  deprecated = false,
}) => {
  const { direction } = useContext(FormContext);
  const theme = useTheme();

  return (
    <div
      css={{
        display: "flex",
        alignItems: "flex-start",
        flexDirection: direction === "horizontal" ? "row" : "column",
        gap: direction === "horizontal" ? 120 : 24,

        [theme.breakpoints.down("md")]: {
          flexDirection: "column",
          gap: 16,
        },
      }}
      className={classes.root}
    >
      <div
        css={{
          width: "100%",
          maxWidth: direction === "horizontal" ? 312 : undefined,
          flexShrink: 0,
          position: direction === "horizontal" ? "sticky" : undefined,
          top: 24,

          [theme.breakpoints.down("md")]: {
            width: "100%",
            position: "initial" as const,
          },
        }}
        className={classes.sectionInfo}
      >
        <h2 css={styles.formSectionInfoTitle} className={classes.infoTitle}>
          {title}
          {alpha && <AlphaBadge />}
          {deprecated && <DeprecatedBadge />}
        </h2>
        <div css={styles.formSectionInfoDescription}>{description}</div>
      </div>

      {children}
    </div>
  );
};

export const FormFields: FC<ComponentProps<typeof Stack>> = (props) => {
  return (
    <Stack
      direction="column"
      spacing={3}
      {...props}
      css={styles.formSectionFields}
    />
  );
};

const styles = {
  formSectionInfoTitle: (theme) => ({
    fontSize: 20,
    color: theme.palette.text.primary,
    fontWeight: 400,
    margin: 0,
    marginBottom: 8,
    display: "flex",
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
  }),

  formSectionInfoDescription: (theme) => ({
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    margin: 0,
  }),

  formSectionFields: {
    width: "100%",
  },
} satisfies Record<string, Interpolation<Theme>>;

export const FormFooter: FC<Exclude<FormFooterProps, "styles">> = (props) => (
  <BaseFormFooter {...props} styles={footerStyles} />
);

const footerStyles = {
  button: (theme) => ({
    minWidth: 184,

    [theme.breakpoints.down("md")]: {
      width: "100%",
    },
  }),

  footer: (theme) => ({
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-start",
    flexDirection: "row-reverse",
    gap: 16,

    [theme.breakpoints.down("md")]: {
      flexDirection: "column",
      gap: 8,
    },
  }),
} satisfies FormFooterStyles;
