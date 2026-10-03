import { beforeEach, describe, expect, it } from "vitest";
import type { CreateTrialLicenseRequest } from "#/api/typesGenerated";
import {
	clearPremiumFunnelAttribution,
	readPremiumFunnelAttribution,
	storePremiumFunnelAttribution,
	trackPremiumFunnelClick,
	withPremiumFunnelAttribution,
} from "./premiumFunnelAttribution";

const attribution = {
	id: "b3f1b3a0-0f5a-4f6a-9c1a-4f2f9b0a7c11",
	source: "appearance",
	createdAt: 1_000_000,
} as const;

describe("premiumFunnelAttribution", () => {
	beforeEach(() => {
		sessionStorage.clear();
	});

	it("round trips a stored attribution", () => {
		storePremiumFunnelAttribution(attribution);

		expect(readPremiumFunnelAttribution(attribution.createdAt)).toEqual(
			attribution,
		);
	});

	it("returns undefined when nothing is stored", () => {
		expect(readPremiumFunnelAttribution()).toBeUndefined();
	});

	it("discards an attribution older than the TTL", () => {
		storePremiumFunnelAttribution(attribution);

		const pastTTL = attribution.createdAt + 30 * 60 * 1000 + 1;
		expect(readPremiumFunnelAttribution(pastTTL)).toBeUndefined();
		expect(readPremiumFunnelAttribution(attribution.createdAt)).toBeUndefined();
	});

	it("keeps an attribution inside the TTL", () => {
		storePremiumFunnelAttribution(attribution);

		const withinTTL = attribution.createdAt + 29 * 60 * 1000;
		expect(readPremiumFunnelAttribution(withinTTL)).toEqual(attribution);
	});

	it("discards malformed storage contents", () => {
		sessionStorage.setItem("coder.premiumFunnelAttribution", "not json");

		expect(readPremiumFunnelAttribution()).toBeUndefined();
	});

	it("discards contents missing required fields", () => {
		sessionStorage.setItem(
			"coder.premiumFunnelAttribution",
			JSON.stringify({ id: attribution.id }),
		);

		expect(readPremiumFunnelAttribution()).toBeUndefined();
	});

	it("discards a source that is not a known paywall", () => {
		sessionStorage.setItem(
			"coder.premiumFunnelAttribution",
			JSON.stringify({ ...attribution, source: "marketing_email" }),
		);

		expect(readPremiumFunnelAttribution(attribution.createdAt)).toBeUndefined();
	});

	it("clears a stored attribution", () => {
		storePremiumFunnelAttribution(attribution);
		clearPremiumFunnelAttribution();

		expect(readPremiumFunnelAttribution(attribution.createdAt)).toBeUndefined();
	});
});

describe("trackPremiumFunnelClick", () => {
	beforeEach(() => {
		sessionStorage.clear();
	});

	// The trial signup is joined to the click on this ID, so the reported event
	// and the stored attribution must carry the same one.
	it("stores the attribution it reports", () => {
		const event = trackPremiumFunnelClick("audit_log", "small");

		expect(event).toEqual({
			id: expect.any(String),
			source: "audit_log",
			variant: "small",
		});
		expect(readPremiumFunnelAttribution()?.id).toBe(event.id);
	});
});

describe("withPremiumFunnelAttribution", () => {
	const request: CreateTrialLicenseRequest = {
		email: "admin@coder.com",
		first_name: "Ada",
		last_name: "Lovelace",
		phone_number: "+14155552671",
		job_title: "Platform Engineer",
		company_name: "Coder",
		country: "United States",
		developers: "51 - 100",
	};

	beforeEach(() => {
		sessionStorage.clear();
	});

	it("tags the request with the stored attribution", () => {
		expect(withPremiumFunnelAttribution(request, attribution)).toEqual({
			...request,
			source: "appearance",
			attribution_id: attribution.id,
		});
	});

	it("reports direct when no attribution is stored", () => {
		expect(withPremiumFunnelAttribution(request)).toEqual({
			...request,
			source: "direct",
			attribution_id: undefined,
		});
	});
});
