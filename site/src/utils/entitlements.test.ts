import { getFeatureVisibility } from "./entitlements";

describe("getFeatureVisibility", () => {
  it("returns empty object if there is no license", () => {
    const result = getFeatureVisibility(false, {
      audit_log: { entitlement: "entitled", enabled: true },
    });
    expect(result).toEqual(expect.objectContaining({}));
  });
  it("returns false for a feature that is not enabled", () => {
    const result = getFeatureVisibility(true, {
      audit_log: { entitlement: "entitled", enabled: false },
    });
    expect(result).toEqual(expect.objectContaining({ audit_log: false }));
  });
  it("returns false for a feature that is not entitled", () => {
    const result = getFeatureVisibility(true, {
      audit_log: { entitlement: "not_entitled", enabled: true },
    });
    expect(result).toEqual(expect.objectContaining({ audit_log: false }));
  });
  it("returns true for a feature that is in grace period", () => {
    const result = getFeatureVisibility(true, {
      audit_log: { entitlement: "grace_period", enabled: true },
    });
    expect(result).toEqual(expect.objectContaining({ audit_log: true }));
  });
  it("returns true for a feature that is in entitled", () => {
    const result = getFeatureVisibility(true, {
      audit_log: { entitlement: "entitled", enabled: true },
    });
    expect(result).toEqual(expect.objectContaining({ audit_log: true }));
  });
});
