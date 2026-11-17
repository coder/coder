import {
	MockMultiOrgExceededUserAISpendStatus,
	MockOrganization,
	MockOrganization2,
	MockUserAISpendStatus,
} from "#/testHelpers/entities";
import { getViewedSpend } from "./viewedSpend";

describe("getViewedSpend", () => {
	const multiOrg = MockMultiOrgExceededUserAISpendStatus;

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
			getViewedSpend(multiOrg, selectedOrgId).shownOrgSpend?.organization.id,
		).toBe(expectedOrgId);
	});

	it("falls back to the first entry when organization_id matches none", () => {
		const { shownOrgSpend } = getViewedSpend(
			{
				...multiOrg,
				organization_id: null,
				effective_group_id: null,
				effective_budget: null,
				current_spend_micros: 0,
			},
			undefined,
		);
		expect(shownOrgSpend?.organization.id).toBe(MockOrganization.id);
	});

	const { organizations: _, ...mockAISpendWithoutOrganizations } =
		MockUserAISpendStatus;
	it.each<[string, Parameters<typeof getViewedSpend>[0]]>([
		[
			"a single organization",
			{
				...MockUserAISpendStatus,
				organizations: [
					{
						...MockUserAISpendStatus.organizations[0],
						current_spend_micros: 1,
					},
				],
			},
		],
		["a server that omits organizations", mockAISpendWithoutOrganizations],
	])("shows the top-level spend without a selector for %s", (_name, data) => {
		expect(getViewedSpend(data, MockOrganization.id)).toEqual({
			spend: { currentSpend: 819_000_000, spendLimit: 1_200_000_000 },
		});
	});
});
