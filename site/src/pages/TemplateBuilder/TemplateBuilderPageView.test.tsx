import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import type {
	TemplateBuilderBasesResponse,
	TemplateBuilderModulesResponse,
} from "#/api/typesGenerated";
import { render } from "#/testHelpers/renderHelpers";
import { TemplateBuilderPageView } from "./TemplateBuilderPageView";

const mockBases: TemplateBuilderBasesResponse = {
	bases: [
		{
			id: "docker",
			name: "Docker",
			description: "Provision workspaces as Docker containers.",
			icon: "/icon/docker.png",
			os: "linux",
			variables: [],
			prerequisites: "",
			agents: [],
		},
	],
};

describe(TemplateBuilderPageView.name, () => {
	it("moves focus to each step heading, including one that mounts after loading", async () => {
		vi.spyOn(API, "getTemplateBuilderBases").mockResolvedValue(mockBases);
		const { promise: modules, resolve: resolveModules } =
			Promise.withResolvers<TemplateBuilderModulesResponse>();
		vi.spyOn(API, "getTemplateBuilderModules").mockReturnValue(modules);
		render(
			<TemplateBuilderPageView
				error={null}
				basesData={mockBases}
				onCreateTemplate={vi.fn()}
				createError={null}
				isCreating={false}
				sessionId="session-1"
			/>,
		);

		await userEvent.click(await screen.findByRole("radio", { name: /Docker/ }));
		await userEvent.click(screen.getByRole("button", { name: "Continue" }));
		resolveModules({ modules: [] });
		await waitFor(() =>
			expect(
				screen.getByRole("heading", { name: "Select modules" }),
			).toHaveFocus(),
		);

		await userEvent.click(screen.getByRole("button", { name: "Back" }));
		await waitFor(() =>
			expect(
				screen.getByRole("heading", { name: "Pick a base template" }),
			).toHaveFocus(),
		);
	});
});
