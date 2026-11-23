import {
	MockMultiOrgExceededUserOrganizationsAISpendStatus,
	MockOrganization,
	MockOrganization2,
	MockUserAISpendStatus,
	MockUserOrganizationsAISpendStatus,
} from "#/testHelpers/entities";
import { getUserMenuSpend } from "./viewedSpend";

describe("getUserMenuSpend", () => {
	const multiOrg = MockMultiOrgExceededUserOrganizationsAISpendStatus;

	it.each([
		[
			"the organization_id entry when nothing is selected",
			undefined,
			MockOrganization2.id,
		],
		[
			"the organization_id entry when the selection disappeared",
			"removed-org-id",
			MockOrganization2.id,
		],
	])("shows %s", (_name, selectedOrgId, expectedOrgId) => {
		expect(
			getUserMenuSpend(MockUserAISpendStatus, multiOrg, selectedOrgId)
				?.orgSelection?.shown.organization.id,
		).toBe(expectedOrgId);
	});

	it("falls back to the first entry when organization_id matches none", () => {
		const viewed = getUserMenuSpend(
			MockUserAISpendStatus,
			{ ...multiOrg, organization_id: null },
			undefined,
		);
		expect(viewed?.orgSelection?.shown.organization.id).toBe(
			MockOrganization.id,
		);
	});

	it.each([
		[
			"a single organization",
			{
				...MockUserOrganizationsAISpendStatus,
				organizations: [
					{
						...MockUserOrganizationsAISpendStatus.organizations[0],
						current_spend_micros: 1,
					},
				],
			},
		],
		["a per-organization request without data", undefined],
	])("shows the site-wide spend without a selector for %s", (_name, byOrg) => {
		expect(
			getUserMenuSpend(MockUserAISpendStatus, byOrg, MockOrganization.id),
		).toEqual({
			spend: { currentSpend: 819_000_000, spendLimit: 1_200_000_000 },
			period: MockUserAISpendStatus,
		});
	});
});
