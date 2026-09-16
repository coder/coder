import { QueryClient } from "react-query";
import { expect, it } from "vitest";
import type { OrganizationAISpendReport } from "#/api/typesGenerated";
import { MockOrganizationAISpendReport } from "#/testHelpers/entities";
import {
	organizationAISpendKey,
	paginatedOrganizationAISpend,
} from "./aiBridge";

const filter = { provider_name: "openai" };

const placeholderAfter = (
	previousKey: readonly unknown[],
): OrganizationAISpendReport | undefined => {
	const { placeholderData } = paginatedOrganizationAISpend("org-a", filter);
	const client = new QueryClient();
	const previousQuery = client
		.getQueryCache()
		.build<
			OrganizationAISpendReport,
			unknown,
			OrganizationAISpendReport,
			readonly unknown[]
		>(client, { queryKey: previousKey });
	return typeof placeholderData === "function"
		? placeholderData(MockOrganizationAISpendReport, previousQuery)
		: placeholderData;
};

it("keeps the previous spend report only across page changes", () => {
	expect(placeholderAfter(organizationAISpendKey("org-a", filter, 2))).toBe(
		MockOrganizationAISpendReport,
	);
	expect(placeholderAfter(organizationAISpendKey("org-a", {}, 1))).toBe(
		undefined,
	);
	expect(placeholderAfter(organizationAISpendKey("org-b", filter, 1))).toBe(
		undefined,
	);
});
