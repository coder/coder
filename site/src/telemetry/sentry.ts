/**
 * Sentry-backed sink for client error reporting. This module is loaded
 * exclusively via dynamic import, and only when the deployment renders a
 * sentry-config metadata tag. Deployments without the config never fetch
 * this chunk or the Sentry SDK.
 */
import * as Sentry from "@sentry/react";
import type { BuildInfoResponse } from "#/api/typesGenerated";
import type { ClientErrorReportingConfig } from "#/hooks/useEmbeddedMetadata";
import { registerClientErrorSink } from "#/utils/clientErrorReporting";

// Integrations that capture uncaught exceptions and instrument browser
// APIs globally. Reporting is intentionally limited to explicit
// reportClientError call sites, so these are removed from the defaults.
const GLOBAL_CAPTURE_INTEGRATIONS = ["GlobalHandlers", "BrowserApiErrors"];

export const init = (
	config: ClientErrorReportingConfig,
	buildInfo?: BuildInfoResponse,
): void => {
	Sentry.init({
		dsn: config.dsn,
		environment: config.environment,
		release: buildInfo?.version,
		// No user identification, request bodies, or headers.
		sendDefaultPii: false,
		// Errors only: no tracing, no session replay.
		tracesSampleRate: 0,
		integrations: (defaults) =>
			defaults.filter(
				(integration) =>
					!GLOBAL_CAPTURE_INTEGRATIONS.includes(integration.name),
			),
		// Console breadcrumbs can contain chat content; drop them entirely.
		beforeBreadcrumb: (breadcrumb) =>
			breadcrumb.category === "console" ? null : breadcrumb,
		maxValueLength: 2048,
	});

	registerClientErrorSink((error, context) => {
		Sentry.captureException(error, { extra: context });
	});
};
