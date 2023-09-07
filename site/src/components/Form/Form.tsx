import { makeStyles } from "@mui/styles";
import {
  FormFooterProps as BaseFormFooterProps,
  FormFooter as BaseFormFooter,
} from "components/FormFooter/FormFooter";
import { Stack } from "components/Stack/Stack";
import {
  createContext,
  FC,
  HTMLProps,
  PropsWithChildren,
  useContext,
} from "react";
import { combineClasses } from "utils/combineClasses";

type FormContextValue = { direction?: "horizontal" | "vertical" };

const FormContext = createContext<FormContextValue>({
  direction: "horizontal",
});

type FormProps = HTMLProps<HTMLFormElement> & {
  direction?: FormContextValue["direction"];
};

export const Form: FC<FormProps> = ({ direction, className, ...formProps }) => {
  const styles = useStyles({ direction });

  return (
    <FormContext.Provider value={{ direction }}>
      <form
        {...formProps}
        className={combineClasses([styles.form, className])}
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
  }
> = ({ children, title, description, classes = {} }) => {
  const formContext = useContext(FormContext);
  const styles = useStyles(formContext);

  return (
    <div className={combineClasses([styles.formSection, classes.root])}>
      <div
        className={combineClasses([
          classes.sectionInfo,
          styles.formSectionInfo,
        ])}
      >
        <h2
          className={combineClasses([
            styles.formSectionInfoTitle,
            classes.infoTitle,
          ])}
        >
          {title}
        </h2>
        <div className={styles.formSectionInfoDescription}>{description}</div>
      </div>

      {children}
    </div>
  );
};

export const FormFields: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles();
  return (
    <Stack
      direction="column"
      spacing={2.5}
      className={styles.formSectionFields}
    >
      {children}
    </Stack>
  );
};

export const FormFooter: FC<BaseFormFooterProps> = (props) => {
  const formFooterStyles = useFormFooterStyles();
  return (
    <BaseFormFooter
      {...props}
      styles={{ ...formFooterStyles, ...props.styles }}
    />
  );
};
const getFlexDirection = ({ direction }: FormContextValue = {}):
  | "row"
  | "column" =>
  direction === "horizontal" ? ("row" as const) : ("column" as const);

const useStyles = makeStyles((theme) => ({
  form: {
    display: "flex",
    flexDirection: "column",
    gap: ({ direction }: FormContextValue = {}) =>
      direction === "horizontal" ? theme.spacing(10) : theme.spacing(5),

    [theme.breakpoints.down("md")]: {
      gap: theme.spacing(8),
    },
  },

  formSection: {
    display: "flex",
    alignItems: "flex-start",
    gap: ({ direction }: FormContextValue = {}) =>
      direction === "horizontal" ? theme.spacing(15) : theme.spacing(3),
    flexDirection: getFlexDirection,

    [theme.breakpoints.down("md")]: {
      flexDirection: "column",
      gap: theme.spacing(2),
    },
  },

  formSectionInfo: {
    width: "100%",
    maxWidth: ({ direction }: FormContextValue = {}) =>
      direction === "horizontal" ? 312 : undefined,
    flexShrink: 0,
    position: ({ direction }: FormContextValue = {}) =>
      direction === "horizontal" ? "sticky" : undefined,
    top: theme.spacing(3),

    [theme.breakpoints.down("md")]: {
      width: "100%",
      position: "initial" as const,
    },
  },

  formSectionInfoTitle: {
    fontSize: 20,
    color: theme.palette.text.primary,
    fontWeight: 400,
    margin: 0,
    marginBottom: theme.spacing(1),
  },

  formSectionInfoDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    margin: 0,
  },

  formSectionFields: {
    width: "100%",
  },
}));

const useFormFooterStyles = makeStyles((theme) => ({
  button: {
    minWidth: theme.spacing(23),

    [theme.breakpoints.down("md")]: {
      width: "100%",
    },
  },
  footer: {
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-start",
    flexDirection: "row-reverse",
    gap: theme.spacing(2),

    [theme.breakpoints.down("md")]: {
      flexDirection: "column",
      gap: theme.spacing(1),
    },
  },
}));
