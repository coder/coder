import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { LoadingButton } from "../LoadingButton/LoadingButton"

export const Language = {
  cancelLabel: "Cancel",
  defaultSubmitLabel: "Submit",
}

export interface FormFooterProps {
  onCancel: () => void
  isLoading: boolean
  submitLabel?: string
  submitDisabled?: boolean
}

const useStyles = makeStyles((theme) => ({
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

export const FormFooter: FC<React.PropsWithChildren<FormFooterProps>> = ({
  onCancel,
  isLoading,
  submitLabel = Language.defaultSubmitLabel,
  submitDisabled,
}) => {
  const styles = useStyles()
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
