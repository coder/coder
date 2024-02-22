import Button from "@mui/material/Button";
import { type FC } from "react";
import { Interpolation, Theme } from "@emotion/react";
import LoadingButton from "@mui/lab/LoadingButton";

export const Language = {
  cancelLabel: "Cancel",
  defaultSubmitLabel: "Submit",
};

export interface FormFooterStyles {
  footer: Interpolation<Theme>;
  button: Interpolation<Theme>;
}

export interface FormFooterProps {
  onCancel: () => void;
  isLoading: boolean;
  styles?: FormFooterStyles;
  submitLabel?: string;
  submitDisabled?: boolean;
  extraActions?: React.ReactNode;
}

export const FormFooter: FC<FormFooterProps> = ({
  onCancel,
  extraActions,
  isLoading,
  submitDisabled,
  submitLabel = Language.defaultSubmitLabel,
  styles = defaultStyles,
}) => {
  return (
    <div css={styles.footer}>
      <LoadingButton
        size="large"
        tabIndex={0}
        loading={isLoading}
        css={styles.button}
        variant="contained"
        color="primary"
        type="submit"
        disabled={submitDisabled}
        data-testid="form-submit"
      >
        {submitLabel}
      </LoadingButton>
      <Button
        size="large"
        type="button"
        css={styles.button}
        onClick={onCancel}
        tabIndex={0}
      >
        {Language.cancelLabel}
      </Button>
      {extraActions}
    </div>
  );
};

const defaultStyles = {
  footer: {
    display: "flex",
    flex: "0",
    // The first button is the submit so it is the first element to be focused
    // on tab so we use row-reverse to display it on the right
    flexDirection: "row-reverse",
    gap: 12,
    alignItems: "center",
    marginTop: 24,
  },
  button: {
    width: "100%",
  },
} satisfies FormFooterStyles;
