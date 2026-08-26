import dayjs from "dayjs";
import { selectTrialCta } from "./UserDropdownPremiumTrialCTA";

describe("selectTrialCta", () => {
	const now = dayjs("2026-08-25T12:00:00Z").valueOf();
	const admin = { canViewLicenses: true, now };
	const inDays = (days: number) => dayjs(now).add(days, "day").unix();
	const inHours = (hours: number) => dayjs(now).add(hours, "hour").unix();

	it("offers the trial when the deployment has no license", () => {
		expect(
			selectTrialCta({
				...admin,
				hasLicense: false,
				isTrial: false,
				trialExpiresAt: undefined,
			}),
		).toEqual({ kind: "start", days: 30 });
	});

	it("counts down the days left on an active trial", () => {
		expect(
			selectTrialCta({
				...admin,
				hasLicense: true,
				isTrial: true,
				trialExpiresAt: inDays(3),
			}),
		).toEqual({ kind: "daysLeft", days: 3 });
	});

	it("reports the final partial day as expiring today", () => {
		expect(
			selectTrialCta({
				...admin,
				hasLicense: true,
				isTrial: true,
				trialExpiresAt: inHours(18),
			}),
		).toEqual({ kind: "expiresToday", days: 0 });
	});

	it("hides a trial that has already expired", () => {
		expect(
			selectTrialCta({
				...admin,
				hasLicense: true,
				isTrial: true,
				trialExpiresAt: inDays(-2),
			}),
		).toBeUndefined();
	});

	it("hides a trial whose expiry is not yet known", () => {
		expect(
			selectTrialCta({
				...admin,
				hasLicense: true,
				isTrial: true,
				trialExpiresAt: undefined,
			}),
		).toBeUndefined();
	});

	it("hides a trial whose expiry is not a finite number", () => {
		expect(
			selectTrialCta({
				...admin,
				hasLicense: true,
				isTrial: true,
				trialExpiresAt: Number.NaN,
			}),
		).toBeUndefined();
	});

	it("hides the entry once a non-trial license is installed", () => {
		expect(
			selectTrialCta({
				...admin,
				hasLicense: true,
				isTrial: false,
				trialExpiresAt: undefined,
			}),
		).toBeUndefined();
	});

	it("hides every state from users who cannot read licenses", () => {
		const states = [
			{ hasLicense: false, isTrial: false, trialExpiresAt: undefined },
			{ hasLicense: true, isTrial: true, trialExpiresAt: inDays(3) },
			{ hasLicense: true, isTrial: false, trialExpiresAt: undefined },
		];

		for (const state of states) {
			expect(
				selectTrialCta({ ...state, canViewLicenses: false, now }),
			).toBeUndefined();
		}
	});

	it("pluralizes a single remaining day", () => {
		expect(
			selectTrialCta({
				...admin,
				hasLicense: true,
				isTrial: true,
				trialExpiresAt: inDays(1).valueOf(),
			}),
		).toEqual({ kind: "daysLeft", days: 1 });
	});
});
