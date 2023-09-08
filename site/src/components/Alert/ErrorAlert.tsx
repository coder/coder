import { AlertProps, Alert, AlertDetail } from "./Alert";
import AlertTitle from "@mui/material/AlertTitle";
import { getErrorMessage, getErrorDetail } from "api/errors";
import { FC } from "react";

export const ErrorAlert: FC<
  Omit<AlertProps, "severity" | "children"> & { error: unknown }
> = ({ error, ...alertProps }) => {
  const message = getErrorMessage(error, "Something went wrong.");
  const detail = getErrorDetail(error);

  // For some reason, the message and detail can be the same on the BE, but does
  // not make sense in the FE to showing them duplicated
  const shouldDisplayDetail = message !== detail;

  return (
    <Alert severity="error" {...alertProps}>
      {detail ? (
        <>
          <AlertTitle>{message}</AlertTitle>
          {shouldDisplayDetail && <AlertDetail>{detail}</AlertDetail>}
        </>
      ) : (
        message
      )}
    </Alert>
  );
};
