import AlertTitle from "@mui/material/AlertTitle";
import { getErrorDetail, getErrorMessage, getErrorStatus } from "api/errors";
import type { FC } from "react";
import { Link } from "../Link/Link";
import { Alert, AlertDetail, type AlertProps } from "./Alert";

export const ErrorAlert: FC<
	Omit<AlertProps, "severity" | "children"> & { error: unknown }
> = ({ error, ...alertProps }) => {
	const message = getErrorMessage(error, "Something went wrong.");
	const detail = getErrorDetail(error);
	const status = getErrorStatus(error);

	// For some reason, the message and detail can be the same on the BE, but does
	// not make sense in the FE to showing them duplicated
	const shouldDisplayDetail = message !== detail;

	return (
		<Alert severity="error" {...alertProps}>
			{
				// When the error is a Forbidden response we include a link for the user to
				// go back to a known viewable page.
				status === 403 ? (
					<>
						<AlertTitle>{message}</AlertTitle>
						<AlertDetail>
							{detail}{" "}
							<Link href="/workspaces" className="w-fit">
								Go to workspaces
							</Link>
						</AlertDetail>
					</>
				) : detail ? (
					<>
						<AlertTitle>{message}</AlertTitle>
						{shouldDisplayDetail && <AlertDetail>{detail}</AlertDetail>}
					</>
				) : (
					message
				)
			}
		</Alert>
	);
};
