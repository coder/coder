import { AlertProps, Alert, AlertDetail } from "./Alert"
import AlertTitle from "@mui/material/AlertTitle"
import { getErrorMessage, getErrorDetail } from "api/errors"
import { FC } from "react"

export const ErrorAlert: FC<
  Omit<AlertProps, "severity" | "children"> & { error: unknown }
> = ({ error, ...alertProps }) => {
  const message = getErrorMessage(error, "Something went wrong.")
  const detail = getErrorDetail(error)

  return (
    <Alert severity="error" {...alertProps}>
      {detail ? (
        <>
          <AlertTitle>{message}</AlertTitle>
          <AlertDetail>{detail}</AlertDetail>
        </>
      ) : (
        message
      )}
    </Alert>
  )
}
