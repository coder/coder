import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import {
  createContext,
  type FC,
  type HTMLProps,
  type PropsWithChildren,
  useContext,
} from "react";
import { AlphaBadge } from "components/DeploySettingsLayout/Badges";
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

export const FormSection: FC<
  PropsWithChildren & {
    title: string | JSX.Element;
    description: string | JSX.Element;
    classes?: {
      root?: string;
      sectionInfo?: string;
      infoTitle?: string;
    };
    alpha?: boolean;
  }
> = ({ children, title, description, classes = {}, alpha = false }) => {
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
        <h2
          css={[
            styles.formSectionInfoTitle,
            alpha && styles.formSectionInfoTitleAlpha,
          ]}
          className={classes.infoTitle}
        >
          {title}
          {alpha && <AlphaBadge />}
        </h2>
        <div css={styles.formSectionInfoDescription}>{description}</div>
      </div>

      {children}
    </div>
  );
};

export const FormFields: FC<PropsWithChildren> = ({ children }) => {
  return (
    <Stack direction="column" spacing={2.5} css={styles.formSectionFields}>
      {children}
    </Stack>
  );
};

const styles = {
  formSectionInfoTitle: (theme) => ({
    fontSize: 20,
    color: theme.palette.text.primary,
    fontWeight: 400,
    margin: 0,
    marginBottom: 8,
  }),

  formSectionInfoTitleAlpha: {
    display: "flex",
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
  },

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

export const FormFooter = (props: Exclude<FormFooterProps, "styles">) => (
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
