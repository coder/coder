import { getFeatureVisibility } from "./entitlements";

describe("getFeatureVisibility", () => {
	it("uses the backend-computed usable state", () => {
		const result = getFeatureVisibility({
			audit_log: {
				entitlement: "entitled",
				enabled: true,
				usable: true,
			},
			user_limit: {
				entitlement: "entitled",
				enabled: true,
				usable: false,
			},
			browser_only: {
				entitlement: "not_entitled",
				enabled: false,
				usable: false,
			},
		});

		expect(result).toEqual({
			audit_log: true,
			user_limit: false,
			browser_only: false,
		});
	});
});
