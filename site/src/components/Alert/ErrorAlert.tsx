import { getErrorDetail, getErrorMessage, getErrorStatus } from "api/errors";
import { isAxiosError } from "axios";
import type { FC } from "react";
import { Link } from "../Link/Link";
import { Alert, AlertDescription, type AlertProps, AlertTitle } from "./Alert";

type ErrorAlertProps = Readonly<
	Omit<AlertProps, "severity" | "children"> & {
		error: unknown;
		showDebugDetail?: boolean;
	}
>;

export const ErrorAlert: FC<ErrorAlertProps> = ({
	error,
	showDebugDetail = true,
	...alertProps
}) => {
	const message = getErrorMessage(error, "Something went wrong.");
	const detail = getErrorDetail(error);
	const status = getErrorStatus(error);

	// For some reason, the message and detail can be the same on the BE, but does
	// not make sense in the FE to showing them duplicated. However, we should always
	// display the detail if its a 403 Forbidden response.
	const shouldDisplayDetail = status === 403 || message !== detail;
	const shouldDisplayResponseData = isAxiosError(error) && error.response?.data;
	const shouldDisplayStackTrace = error instanceof Error;

	return (
		<Alert severity="error" prominent {...alertProps}>
			<AlertTitle>{message}</AlertTitle>
			<AlertDescription>
				{shouldDisplayDetail && detail}
				{status === 403 && (
					// When the error is a Forbidden response we include a link for the user to
					// go back to a known viewable page.
					<Link href="/workspaces" className="w-fit">
						Go to workspaces
					</Link>
				)}
			</AlertDescription>
			{(shouldDisplayResponseData || shouldDisplayStackTrace) &&
				showDebugDetail && (
					<div className="mt-2 min-w-0">
						{shouldDisplayResponseData && (
							<details className="max-w-full">
								<summary>Response data</summary>
								<div className="mt-2 max-w-full overflow-x-auto">
									<pre className="m-0 w-max min-w-full">
										{JSON.stringify(error.response?.data, null, 2)}
									</pre>
								</div>
							</details>
						)}
						{/*
						 * Error.isError() is not reliably available in all browsers
						 * so we fallback to `instanceof Error`. In future we should use
						 * it is more reliable.
						 */}
						{shouldDisplayStackTrace && (
							<details className="max-w-full">
								<summary>Stack Trace</summary>
								<div className="mt-2 max-w-full overflow-x-auto">
									<pre className="m-0 w-max min-w-full">{error.stack}</pre>
								</div>
							</details>
						)}
					</div>
				)}
		</Alert>
	);
};
