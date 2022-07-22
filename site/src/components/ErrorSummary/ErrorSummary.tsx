import Button from "@material-ui/core/Button"
import Collapse from "@material-ui/core/Collapse"
import IconButton from "@material-ui/core/IconButton"
import Link from "@material-ui/core/Link"
import { makeStyles, Theme } from "@material-ui/core/styles"
import CloseIcon from "@material-ui/icons/Close"
import RefreshIcon from "@material-ui/icons/Refresh"
import { ApiError, getErrorDetail, getErrorMessage } from "api/errors"
import { Stack } from "components/Stack/Stack"
import { FC, useState } from "react"

const Language = {
  retryMessage: "Retry",
  unknownErrorMessage: "An unknown error has occurred",
  moreDetails: "More",
  lessDetails: "Less",
}

export interface ErrorSummaryProps {
  error: ApiError | Error | unknown
  retry?: () => void
  dismissible?: boolean
  defaultMessage?: string
}

export const ErrorSummary: FC<ErrorSummaryProps> = ({
  error,
  retry,
  dismissible,
  defaultMessage,
}) => {
  const message = getErrorMessage(error, defaultMessage || Language.unknownErrorMessage)
  const detail = getErrorDetail(error)
  const [showDetails, setShowDetails] = useState(false)
  const [isOpen, setOpen] = useState(true)

  const styles = useStyles({ showDetails })

  const toggleShowDetails = () => {
    setShowDetails(!showDetails)
  }

  const closeError = () => {
    setOpen(false)
  }

  if (!isOpen) {
    return null
  }

  return (
    <Stack>
      <Stack className={styles.root}>
        <Stack direction="row" alignItems="center" className={styles.message}>
          <div>
            <span className={styles.errorMessage}>{message}</span>
            {!!detail && (
              <Link
                aria-expanded={showDetails}
                onClick={toggleShowDetails}
                className={styles.detailsLink}
              >
                {showDetails ? Language.lessDetails : Language.moreDetails}
              </Link>
            )}
          </div>
          {dismissible && (
            <IconButton onClick={closeError} className={styles.iconButton}>
              <CloseIcon className={styles.closeIcon} />
            </IconButton>
          )}
        </Stack>
        <Collapse in={showDetails}>{detail}</Collapse>
      </Stack>

      {retry && (
        <div>
          <Button onClick={retry} startIcon={<RefreshIcon />} variant="outlined">
            {Language.retryMessage}
          </Button>
        </div>
      )}
    </Stack>
  )
}

interface StyleProps {
  showDetails?: boolean
}

const useStyles = makeStyles<Theme, StyleProps>((theme) => ({
  root: {
    background: `${theme.palette.error.main}60`,
    margin: `${theme.spacing(2)}px`,
    padding: `${theme.spacing(2)}px`,
    borderRadius: theme.shape.borderRadius,
    gap: (props) => (props.showDetails ? `${theme.spacing(2)}px` : 0),
  },
  message: {
    justifyContent: "space-between",
  },
  errorMessage: {
    marginRight: `${theme.spacing(1)}px`,
  },
  detailsLink: {
    cursor: "pointer",
  },
  iconButton: {
    padding: 0,
  },
  closeIcon: {
    width: 25,
    height: 25,
    color: theme.palette.primary.contrastText,
  },
}))
