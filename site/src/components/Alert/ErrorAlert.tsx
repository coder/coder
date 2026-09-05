import { isAxiosError } from "axios";
import type { FC } from "react";
import { getErrorDetail, getErrorMessage, getErrorStatus } from "#/api/errors";
import { Link } from "../Link/Link";
import { Alert, AlertDescription, type AlertProps, AlertTitle } from "./Alert";

type ErrorAlertProps = Readonly<
	Omit<AlertProps, "severity" | "children"> & {
		error: unknown;
		showDebugDetail?: boolean;
	}
>;

// Some responses use a raw HTTP status word as the message and put the
// actionable explanation in the detail. Those status words are meaningless to
// users, so they are replaced with a human title and the detail carries the
// explanation.
const genericStatusTitles: Record<number, { pattern: RegExp; title: string }> =
	{
		403: { pattern: /^forbidden\.?$/i, title: "Permission required" },
	};

export const ErrorAlert: FC<ErrorAlertProps> = ({
	error,
	showDebugDetail = true,
	...alertProps
}) => {
	const message = getErrorMessage(error, "Something went wrong.");
	const detail = getErrorDetail(error);
	const status = getErrorStatus(error);
	const isForbidden = status === 403;

	const genericStatus = status ? genericStatusTitles[status] : undefined;
	const title = genericStatus?.pattern.test(message.trim())
		? genericStatus.title
		: message;

	// For some reason, the message and detail can be the same on the BE, but does
	// not make sense in the FE to showing them duplicated.
	const shouldDisplayDetail = detail !== undefined && detail !== title;
	const shouldDisplayResponseData = isAxiosError(error) && error.response?.data;
	const shouldDisplayStackTrace = error instanceof Error;

	return (
		<Alert severity="error" prominent {...alertProps}>
			<AlertTitle className="font-semibold">{title}</AlertTitle>
			<AlertDescription>
				<span className="flex flex-col items-start gap-1">
					{shouldDisplayDetail && <span>{detail}</span>}
					{isForbidden && (
						// When the error is a Forbidden response we include a link for the user to
						// go back to a known viewable page.
						<Link href="/workspaces" className="w-fit">
							Go to workspaces
						</Link>
					)}
				</span>
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
