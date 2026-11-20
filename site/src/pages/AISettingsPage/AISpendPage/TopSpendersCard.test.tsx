import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { expect, it } from "vitest";
import { MockOrganizationAISpendOverview } from "#/testHelpers/entities";
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
						canViewSessions
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
