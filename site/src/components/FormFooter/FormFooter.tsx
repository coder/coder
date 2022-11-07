import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { ClassNameMap } from "@material-ui/core/styles/withStyles"
import { FC } from "react"
import { LoadingButton } from "../LoadingButton/LoadingButton"

export const Language = {
  cancelLabel: "Cancel",
  defaultSubmitLabel: "Submit",
}

type FormFooterStyles = ClassNameMap<"footer" | "button">
export interface FormFooterProps {
  onCancel: () => void
  isLoading: boolean
  styles?: FormFooterStyles
  submitLabel?: string
  submitDisabled?: boolean
}

export const FormFooter: FC<FormFooterProps> = ({
  onCancel,
  isLoading,
  submitDisabled,
  submitLabel = Language.defaultSubmitLabel,
  styles = defaultStyles(),
}) => {
  return (
    <div className={styles.footer}>
      <LoadingButton
        tabIndex={0}
        loading={isLoading}
        className={styles.button}
        variant="contained"
        color="primary"
        type="submit"
        disabled={submitDisabled}
      >
        {submitLabel}
      </LoadingButton>
      <Button
        type="button"
        className={styles.button}
        onClick={onCancel}
        variant="outlined"
        tabIndex={0}
      >
        {Language.cancelLabel}
      </Button>
    </div>
  )
}

const defaultStyles = makeStyles((theme) => ({
  footer: {
    display: "flex",
    flex: "0",
    // The first button is the submit so it is the first element to be focused
    // on tab so we use row-reverse to display it on the right
    flexDirection: "row-reverse",
    gap: theme.spacing(1.5),
    alignItems: "center",
    marginTop: theme.spacing(3),
  },
  button: {
    width: "100%",
  },
}))
