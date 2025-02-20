import AlertTitle from "@mui/material/AlertTitle";
import { getErrorDetail, getErrorMessage, getErrorStatus } from "api/errors";
import { Link } from "components/Link/Link";
import type { FC } from "react";
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

	const body = () => {
		// When the error is a Forbidden response we include a link for the user to
		// go back to a known viewable page.
		// Additionally since the error messages and details from the server can be
		// missing or confusing for an end user we render a friendlier message
		// regardless of the response from the server.
		if (status === 403) {
			return (
				<>
					<AlertTitle>You don't have permission to view this page</AlertTitle>
					<AlertDetail>
						If you believe this is a mistake, please contact your administrator
						or try signing in with different credentials.{" "}
						<Link href="/workspaces" className="w-fit">
							Go to workspaces
						</Link>
					</AlertDetail>
				</>
			);
		}

		if (detail) {
			return (
				<>
					<AlertTitle>{message}</AlertTitle>
					{shouldDisplayDetail && <AlertDetail>{detail}</AlertDetail>}
				</>
			);
		}

		return message;
	};

	return (
		<Alert severity="error" {...alertProps}>
			{body()}
		</Alert>
	);
};
