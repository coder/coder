import { isAxiosError } from "axios";
import { getErrorDetail, getErrorMessage, getErrorStatus } from "#/api/errors";
import { Link } from "../Link/Link";
import {
	Alert,
	AlertDescription,
	type AlertDetail,
	type AlertProps,
	AlertTitle,
} from "./Alert";

type ErrorAlertProps = Readonly<
	Omit<AlertProps, "severity" | "children" | "details"> & {
		error: unknown;
		showDebugDetail?: boolean;
	}
>;

export const ErrorAlert: React.FC<ErrorAlertProps> = ({
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

	return (
		<Alert
			severity="error"
			prominent
			details={showDebugDetail ? getTechnicalDetails(error) : undefined}
			{...alertProps}
		>
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
		</Alert>
	);
};

const headerString = (value: unknown): string | undefined =>
	typeof value === "string" && value !== "" ? value : undefined;

// Response fields other than message and detail, which the alert already shows.
const extraResponseData = (data: unknown): string | undefined => {
	if (typeof data !== "object" || data === null) {
		return undefined;
	}
	const extras = Object.entries(data).filter(
		([key, value]) =>
			key !== "message" &&
			key !== "detail" &&
			value !== undefined &&
			value !== null,
	);
	return extras.length > 0
		? JSON.stringify(Object.fromEntries(extras), null, 2)
		: undefined;
};

const getTechnicalDetails = (error: unknown): AlertDetail[] => {
	const rows: AlertDetail[] = [];

	if (isAxiosError(error)) {
		const { config, response } = error;
		const requestId = headerString(response?.headers?.["x-coder-request-id"]);
		const time = headerString(response?.headers?.date);
		const responseData = extraResponseData(response?.data);
		if (requestId) {
			rows.push({ label: "Request ID", value: requestId });
		}
		if (response?.status) {
			rows.push({
				label: "Status",
				value: `${response.status} ${response.statusText ?? ""}`.trim(),
			});
		}
		if (config?.url) {
			rows.push({
				label: "Request",
				value: `${config.method?.toUpperCase() ?? ""} ${config.url}`.trim(),
			});
		}
		if (time) {
			rows.push({ label: "Time", value: time });
		}
		if (responseData) {
			rows.push({ label: "Response", value: responseData });
		}
	} else if (error instanceof Error && error.stack) {
		// A stack trace only helps when the interface itself failed, not a request.
		rows.push({ label: "Stack", value: error.stack });
	}

	return rows;
};
