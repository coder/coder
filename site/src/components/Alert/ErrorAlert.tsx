import { AlertProps, Alert } from "./Alert"
import AlertTitle from "@mui/material/AlertTitle"
import Box from "@mui/material/Box"
import { getErrorMessage, getErrorDetail } from "api/errors"
import { FC } from "react"

export const ErrorAlert: FC<
  Omit<AlertProps, "severity" | "children"> & { error: unknown }
> = ({ error, ...alertProps }) => {
  const message = getErrorMessage(error, "An error occurred")
  const detail = getErrorDetail(error)

  return (
    <Alert severity="error" {...alertProps}>
      {detail ? (
        <>
          <AlertTitle>{message}</AlertTitle>
          <Box component="span" color={(theme) => theme.palette.text.secondary}>
            {detail}
          </Box>
        </>
      ) : (
        message
      )}
    </Alert>
  )
}
