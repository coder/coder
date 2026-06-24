/**
 * Frontend-only types describing network activity captured during an AI
 * session. The backend does not currently surface this data, so these types
 * are decoupled from any generated API model. They exist purely to drive the
 * visual validation of the Network section and the Network activity dialog.
 */

type NetworkRequestStatus = "allowed" | "blocked";

export type HttpMethod =
	| "GET"
	| "POST"
	| "PUT"
	| "PATCH"
	| "DELETE"
	| "HEAD"
	| "OPTIONS";

interface NetworkEventBase {
	id: string;
	timestamp: Date;
	policy?: string;
	error?: string;
	policyConfigurationHref?: string;
}

/**
 * A single intercepted HTTP request. `allowed` requests succeeded against the
 * configured policy, `blocked` requests were stopped before reaching their
 * destination.
 */
export interface NetworkRequestEvent extends NetworkEventBase {
	kind: "request";
	method: HttpMethod;
	status: NetworkRequestStatus;
	url: string;
}

/**
 * A non-request failure that affected the network audit itself, such as the
 * boundary recorder dropping events mid-session. Counted as an `error` in the
 * summary.
 */
export interface NetworkFailureEvent extends NetworkEventBase {
	kind: "failure";
	label: string;
	detail: string;
	description?: string;
}

export type NetworkEvent = NetworkRequestEvent | NetworkFailureEvent;

export interface NetworkActivity {
	events: readonly NetworkEvent[];
}

interface NetworkCounts {
	allowed: number;
	warnings: number;
	errors: number;
	total: number;
}

export const computeNetworkCounts = (
	activity: NetworkActivity | undefined,
): NetworkCounts => {
	const counts: NetworkCounts = {
		allowed: 0,
		warnings: 0,
		errors: 0,
		total: 0,
	};

	if (!activity) {
		return counts;
	}

	for (const event of activity.events) {
		counts.total++;
		if (event.kind === "failure") {
			counts.errors++;
		} else if (event.status === "allowed") {
			counts.allowed++;
		} else {
			counts.warnings++;
		}
	}

	return counts;
};
