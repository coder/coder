import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { expect, it, vi } from "vitest";
import {
	MockOrganizationAISpendOverview,
	MockOrganizationAISpendReport,
	MockOrganizationAISpendUser,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { TopSpendersCard } from "./TopSpendersCard";

it("links a user to their AI session logs for the same period", async () => {
	const user = userEvent.setup();
	renderWithRouter(
		createMemoryRouter([
			{
				path: "/",
				element: (
					<TopSpendersCard
						overview={MockOrganizationAISpendOverview}
						topSpenderDetails={undefined}
						topSpenderDetailsError={null}
						organizationName="Coder"
						canViewSessions
						onOpenDetailedReport={vi.fn()}
					/>
				),
			},
		]),
	);

	await user.click(
		await screen.findByRole("button", { name: "Open menu for Alice Liddell" }),
	);
	const link = within(await screen.findByRole("menu")).getByRole("menuitem", {
		name: "User AI session logs",
	});
	const filter = new URL(
		link.getAttribute("href") ?? "",
		"http://localhost",
	).searchParams.get("filter");
	expect(filter).toBe(
		'initiator:alice started_after:"2026-03-06T00:00:00Z" started_before:"2026-03-13T00:00:00Z"',
	);
});

it("counts a model priced after some of its usage as unpriced", async () => {
	renderWithRouter(
		createMemoryRouter([
			{
				path: "/",
				element: (
					<TopSpendersCard
						overview={MockOrganizationAISpendOverview}
						topSpenderDetails={[
							{
								...MockOrganizationAISpendUser,
								model_spend: [
									{
										// Priced now, but some usage came before the price, so it counts.
										name: "claude-opus-4-6",
										cost_micros: 1_500_000, // $1.50
										has_unpriced_usage: true,
									},
									{
										// Never priced, so it counts.
										name: "llama-4-maverick",
										cost_micros: 0,
										has_unpriced_usage: true,
									},
									{
										// Priced for all its usage, so it doesn't count.
										name: "gpt-5.4",
										cost_micros: 1_000_000, // $1.00
										has_unpriced_usage: false,
									},
								],
							},
							MockOrganizationAISpendReport.users[1],
						]}
						topSpenderDetailsError={null}
						organizationName="Coder"
						canViewSessions
						onOpenDetailedReport={vi.fn()}
					/>
				),
			},
		]),
	);

	const warnings = await screen.findAllByRole("button", {
		name: /without pricing/,
	});
	expect(warnings.map((warning) => warning.getAttribute("aria-label"))).toEqual(
		["2 models without pricing"],
	);
});
