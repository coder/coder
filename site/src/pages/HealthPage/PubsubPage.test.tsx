import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, Outlet } from "react-router";
import { API } from "#/api/api";
import { MockHealth, MockHealthSettings } from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import PubsubPage from "./PubsubPage";

it("mutes and unmutes the pubsub health category", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "getHealthSettings").mockResolvedValue({
		...MockHealthSettings,
		dismissed_healthchecks: [],
	});
	const updateSettings = vi
		.spyOn(API, "updateHealthSettings")
		.mockImplementation(async (settings) => settings);
	renderWithRouter(
		createMemoryRouter(
			[
				{
					element: <Outlet context={MockHealth} />,
					children: [{ path: "/health/pubsub", element: <PubsubPage /> }],
				},
			],
			{ initialEntries: ["/health/pubsub"] },
		),
	);

	await user.click(
		await screen.findByRole("button", { name: "Mute warnings" }),
	);
	await waitFor(() =>
		expect(updateSettings).toHaveBeenCalledWith({
			dismissed_healthchecks: ["Pubsub"],
		}),
	);

	await user.click(
		await screen.findByRole("button", { name: "Unmute warnings" }),
	);
	await waitFor(() =>
		expect(updateSettings).toHaveBeenLastCalledWith({
			dismissed_healthchecks: [],
		}),
	);
});
