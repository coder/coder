import { API } from "#/api/api";

export const reportPremiumFunnelEvent = () => ({
	mutationFn: API.reportPremiumFunnelEvent,
});
