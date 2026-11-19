import { screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import {
	MockOrganizationAISpendOverview,
	MockOrganizationAISpendUser,
} from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { CostDetailsDialog } from "./CostDetailsDialog";

it("warns on a partly priced model and shows no pricing for an unpriced one", () => {
	renderComponent(
		<CostDetailsDialog
			open
			onOpenChange={vi.fn()}
			onCloseAutoFocus={vi.fn()}
			user={MockOrganizationAISpendOverview.users[0]}
			details={{
				...MockOrganizationAISpendUser,
				model_spend: [
					{
						// Priced now, but some usage came before the price.
						name: "claude-opus-4-6",
						cost_micros: 1_500_000, // $1.50
						has_unpriced_usage: true,
					},
					{
						// Never priced.
						name: "llama-4-maverick",
						cost_micros: 0,
						has_unpriced_usage: true,
					},
				],
			}}
			detailsError={null}
			period={MockOrganizationAISpendOverview}
			organizationName="Coder"
		/>,
	);

	const models = within(screen.getByRole("region", { name: "Models" }));
	const pricedLater = models.getByText("claude-opus-4-6").closest("li");
	const unpriced = models.getByText("llama-4-maverick").closest("li");
	expect(pricedLater).toHaveTextContent("Some usage has no pricing$1.50");
	expect(unpriced).toHaveTextContent("No pricing");
	expect(unpriced).not.toHaveTextContent("$");
});
