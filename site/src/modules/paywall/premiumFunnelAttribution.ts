import * as Yup from "yup";
import type {
	CreateTrialLicenseRequest,
	PremiumFunnelEventRequest,
	PremiumFunnelSource,
	PremiumFunnelVariant,
} from "#/api/typesGenerated";
import { PremiumFunnelSources } from "#/api/typesGenerated";
import { generateUUID } from "#/utils/random";

const STORAGE_KEY = "coder.premiumFunnelAttribution";

/**
 * How long a stored attribution stays valid. Without an expiry, a token left
 * behind by an abandoned click would attribute an unrelated trial signup made
 * much later in the same tab.
 */
const TTL_MS = 30 * 60 * 1000;

type PremiumFunnelAttribution = {
	/** ID of the cta_click event that produced this attribution. */
	id: string;
	source: PremiumFunnelSource;
	createdAt: number;
};

const attributionSchema: Yup.ObjectSchema<PremiumFunnelAttribution> =
	Yup.object({
		id: Yup.string().required(),
		source: Yup.string<PremiumFunnelSource>()
			.oneOf(PremiumFunnelSources)
			.required(),
		createdAt: Yup.number().required(),
	});

/**
 * Records which paywall sent the user to the premium page. sessionStorage is
 * used instead of a query parameter so the URL stays clean, and instead of
 * router state so the attribution survives a page refresh.
 */
export const storePremiumFunnelAttribution = (
	attribution: PremiumFunnelAttribution,
): void => {
	sessionStorage.setItem(STORAGE_KEY, JSON.stringify(attribution));
};

export const clearPremiumFunnelAttribution = (): void => {
	sessionStorage.removeItem(STORAGE_KEY);
};

/**
 * Records a click on a paywall call to action and returns the event to report.
 * The event ID doubles as the attribution token, so the trial signup that
 * follows can be joined back to this click.
 */
export const trackPremiumFunnelClick = (
	source: PremiumFunnelSource,
	variant: PremiumFunnelVariant,
): PremiumFunnelEventRequest => {
	const id = generateUUID();
	storePremiumFunnelAttribution({ id, source, createdAt: Date.now() });
	return { id, source, variant };
};

/**
 * Reads the stored attribution, discarding anything expired or malformed.
 */
export const readPremiumFunnelAttribution = (
	now: number = Date.now(),
): PremiumFunnelAttribution | undefined => {
	const raw = sessionStorage.getItem(STORAGE_KEY);
	if (!raw) {
		return undefined;
	}

	let attribution: PremiumFunnelAttribution;
	try {
		attribution = attributionSchema.validateSync(JSON.parse(raw));
	} catch {
		clearPremiumFunnelAttribution();
		return undefined;
	}

	if (now - attribution.createdAt > TTL_MS) {
		clearPremiumFunnelAttribution();
		return undefined;
	}
	return attribution;
};

/**
 * Tags a trial request with the paywall that produced it. Requests made
 * without a stored attribution report "direct", so that arriving without a
 * paywall stays distinguishable from missing data.
 */
export const withPremiumFunnelAttribution = (
	request: CreateTrialLicenseRequest,
	attribution = readPremiumFunnelAttribution(),
): CreateTrialLicenseRequest => ({
	...request,
	source: attribution?.source ?? "direct",
	attribution_id: attribution?.id,
});
