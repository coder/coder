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
		/**
		 * User-facing outcome, for example "We couldn't create this workspace".
		 * The detail becomes the description, and the raw message moves to the
		 * technical details unless there is no detail to replace it.
		 */
		title?: string;
		showDebugDetail?: boolean;
	}
>;

export const ErrorAlert: React.FC<ErrorAlertProps> = ({
	error,
	title,
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
	let description: string | undefined;
	if (title) {
		description = detail ?? message;
	} else if (shouldDisplayDetail) {
		description = detail;
	}
	// A custom title hides the raw message, so keep it in the technical details
	// unless the description already shows it. The display fallback above is
	// not a raw message, so it never gets a row.
	const rawMessage = getErrorMessage(error, "");
	const detailsMessage =
		title && description !== message && rawMessage ? rawMessage : undefined;

	return (
		<Alert
			severity="error"
			prominent
			details={
				showDebugDetail ? getTechnicalDetails(error, detailsMessage) : undefined
			}
			{...alertProps}
		>
			<AlertTitle>{title || message}</AlertTitle>
			<AlertDescription>
				{description}
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

const getTechnicalDetails = (
	error: unknown,
	message: string | undefined,
): AlertDetail[] => {
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
		if (message) {
			rows.push({ label: "Message", value: message });
		}
		if (time) {
			rows.push({ label: "Time", value: time });
		}
		if (responseData) {
			rows.push({ label: "Response", value: responseData });
		}
	} else {
		if (message) {
			rows.push({ label: "Message", value: message });
		}
		// A stack trace only helps when the interface itself failed, not a request.
		if (error instanceof Error && error.stack) {
			rows.push({ label: "Stack", value: error.stack });
		}
	}

	return rows;
};
