/**
 * Deterministic, frontend-only mock data for network activity. Pure visual
 * validation, no backend impact. The mock variant is picked from a stable
 * hash of the session ID so different sessions render different states for
 * manual QA on the live page. Storybook stories exercise each state
 * explicitly.
 */

import type {
	HttpMethod,
	NetworkActivity,
	NetworkEvent,
	NetworkFailureEvent,
	NetworkRequestEvent,
} from "./types";

type MockNetworkVariant =
	| "none"
	| "all-allowed"
	| "mixed"
	| "error-only"
	| "mid-session-failure"
	| "many";

const hashString = (value: string): number => {
	let hash = 0;
	for (let i = 0; i < value.length; i++) {
		hash = (hash * 31 + value.charCodeAt(i)) | 0;
	}
	return Math.abs(hash);
};

// Cycle of variants used when generating mock data per session ID. Keeping
// "none" out of the live cycle so the feature is visible on every session;
// Storybook still has an explicit empty story.
const LIVE_VARIANTS: readonly MockNetworkVariant[] = [
	"all-allowed",
	"mixed",
	"error-only",
	"mid-session-failure",
	"many",
];

const pickMockVariantForSession = (sessionId: string): MockNetworkVariant => {
	if (!sessionId) {
		return "mixed";
	}
	return LIVE_VARIANTS[hashString(sessionId) % LIVE_VARIANTS.length];
};

const BASE_TIME = new Date("2026-12-12T15:00:59Z").getTime();
const POLICY = "no-external-apis v1.2 (Org policy)";
const POLICY_HREF =
	"/deployment/ai-gateway/policies/no-external-apis?version=1.2";

const stepTime = (offsetSeconds: number): Date =>
	new Date(BASE_TIME + offsetSeconds * 1000);

const makeAllowed = (
	id: string,
	method: HttpMethod,
	url: string,
	offset: number,
): NetworkRequestEvent => ({
	kind: "request",
	id,
	method,
	status: "allowed",
	url,
	timestamp: stepTime(offset),
	policy: POLICY,
	policyConfigurationHref: POLICY_HREF,
});

const makeBlocked = (
	id: string,
	method: HttpMethod,
	url: string,
	offset: number,
	error = "Destination not in allowlist",
): NetworkRequestEvent => ({
	kind: "request",
	id,
	method,
	status: "blocked",
	url,
	timestamp: stepTime(offset),
	policy: POLICY,
	error,
	policyConfigurationHref: POLICY_HREF,
});

const makeFailure = (
	id: string,
	offset: number,
	overrides: Partial<NetworkFailureEvent> = {},
): NetworkFailureEvent => ({
	kind: "failure",
	id,
	label: "Network Error",
	detail: "Failed mid-session",
	description:
		"Boundaries stopped recording during the session. Network data may be incomplete.",
	timestamp: stepTime(offset),
	policy: POLICY,
	error: "Unexpected key domains at line 14",
	policyConfigurationHref: POLICY_HREF,
	...overrides,
});

const allAllowed: readonly NetworkEvent[] = Array.from({ length: 12 }, (_, i) =>
	makeAllowed(
		`allowed-${i}`,
		i % 3 === 0 ? "GET" : "POST",
		`api.github.com/repos/coder/coder/issues/${1000 + i}`,
		i * 2,
	),
);

const mixed: readonly NetworkEvent[] = [
	makeAllowed("m-1", "POST", "api.github.com/repos/coder/coder", 0),
	makeBlocked("m-2", "GET", "registry.npmjs.org/lodash", 5),
	makeBlocked("m-3", "POST", "hooks.slack.com/services/T01ABCDE/B0123/xyz", 6),
	makeAllowed("m-4", "POST", "api.github.com/repos/coder/coder/pulls", 12),
	makeAllowed("m-5", "POST", "api.github.com/repos/coder/coder/contents", 18),
	makeBlocked("m-6", "POST", "sentry.io/api/0/store/", 24),
	makeFailure("m-7", 30),
];

const errorOnly: readonly NetworkEvent[] = [
	makeFailure("e-1", 0, {
		label: "Network Error",
		detail: "Failed at session start",
		description:
			"Boundaries failed to start. No network activity was recorded for this session.",
		error: "Failed to attach boundary agent",
	}),
];

const midSessionFailure: readonly NetworkEvent[] = [
	makeAllowed("ms-1", "POST", "api.github.com/repos/coder/coder", 0),
	makeAllowed("ms-2", "GET", "api.github.com/repos/coder/coder/contents", 6),
	makeFailure("ms-3", 14),
];

const many: readonly NetworkEvent[] = [
	...Array.from({ length: 14 }, (_, i) =>
		makeAllowed(
			`many-allowed-${i}`,
			i % 2 === 0 ? "POST" : "GET",
			`api.github.com/repos/coder/coder/issues/${2000 + i}`,
			i,
		),
	),
	...Array.from({ length: 5 }, (_, i) =>
		makeBlocked(
			`many-blocked-${i}`,
			"POST",
			`hooks.slack.com/services/T${i}/B${i}/secret-${i}`,
			20 + i,
		),
	),
	makeFailure("many-failure", 30),
];

const variants: Record<MockNetworkVariant, readonly NetworkEvent[]> = {
	none: [],
	"all-allowed": allAllowed,
	mixed,
	"error-only": errorOnly,
	"mid-session-failure": midSessionFailure,
	many,
};

export const mockNetworkActivity = (
	variant: MockNetworkVariant,
): NetworkActivity => ({ events: variants[variant] });

export const mockNetworkActivityForSession = (
	sessionId: string,
): NetworkActivity => mockNetworkActivity(pickMockVariantForSession(sessionId));
