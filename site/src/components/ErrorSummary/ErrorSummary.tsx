import Button from "@material-ui/core/Button"
import RefreshIcon from "@material-ui/icons/Refresh"
import React from "react"
import { Stack } from "../Stack/Stack"

const Language = {
  retryMessage: "Retry",
  unknownErrorMessage: "An unknown error has occurred",
}

export interface ErrorSummaryProps {
  error: Error | unknown
  retry?: () => void
}

export const ErrorSummary: React.FC<ErrorSummaryProps> = ({ error, retry }) => (
  <Stack>
    {!(error instanceof Error) ? <div>{Language.unknownErrorMessage}</div> : <div>{error.toString()}</div>}

    {retry && (
      <div>
        <Button onClick={retry} startIcon={<RefreshIcon />} variant="outlined">
          {Language.retryMessage}
        </Button>
      </div>
    )}
  </Stack>
)
