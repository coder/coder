/**
 * Client error reporting seam. `reportClientError` is safe to call from
 * anywhere in the app: it forwards to a sink registered by the lazily
 * loaded telemetry module and is a no-op when no sink is registered,
 * which is the case for every deployment that has not opted in to
 * client error reporting.
 */

const MAX_CONTEXT_VALUE_LENGTH = 2048;

type ClientErrorContext = Readonly<Record<string, string>>;

type ClientErrorSink = (error: unknown, context: ClientErrorContext) => void;

let sink: ClientErrorSink | undefined;

export const registerClientErrorSink = (newSink: ClientErrorSink): void => {
	sink = newSink;
};

/**
 * Forwards an error and its context to the registered sink, truncating
 * context values so oversized payloads (e.g. raw stream frames) never
 * leave the browser in full.
 */
export const reportClientError = (
	error: unknown,
	context: ClientErrorContext = {},
): void => {
	if (!sink) {
		return;
	}
	const truncated: Record<string, string> = {};
	for (const [key, value] of Object.entries(context)) {
		truncated[key] =
			value.length > MAX_CONTEXT_VALUE_LENGTH
				? `${value.slice(0, MAX_CONTEXT_VALUE_LENGTH)}...[truncated]`
				: value;
	}
	try {
		sink(error, truncated);
	} catch {
		// Reporting must never break the app.
	}
};
