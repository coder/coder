import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { useState } from "react";
import { API } from "#/api/api";
import type {
	TemplateBuilderBasesResponse,
	TemplateBuilderModulesResponse,
} from "#/api/typesGenerated";
import {
	MockOrganization,
	MockProvisioner,
	mockAxiosError,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
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

	it("links a duplicate ID error to the ID field after a failed create", async () => {
		vi.spyOn(API, "getTemplateBuilderModules").mockResolvedValue({
			modules: [],
		});
		server.use(
			http.get("/api/v2/organizations", () =>
				HttpResponse.json([MockOrganization]),
			),
			http.post("/api/v2/authcheck", () =>
				HttpResponse.json({ [MockOrganization.id]: true }),
			),
			http.get("/api/v2/organizations/:org/provisionerdaemons", () =>
				HttpResponse.json([MockProvisioner]),
			),
		);
		const message =
			"A template with this name already exists in the organization.";
		const duplicateIdError = mockAxiosError({ message, status: 409 });

		const FailingCreate: React.FC = () => {
			const [creation, setCreation] = useState<{
				isCreating: boolean;
				error: Error | null;
			}>({ isCreating: false, error: null });
			return (
				<TemplateBuilderPageView
					error={null}
					basesData={mockBases}
					preselectedBase={{
						id: "docker",
						name: "Docker",
						iconUrl: "/icon/docker.png",
						hasParameters: false,
						hasPrerequisites: false,
					}}
					onCreateTemplate={() => {
						setCreation({ isCreating: true, error: null });
						setTimeout(() =>
							setCreation({ isCreating: false, error: duplicateIdError }),
						);
					}}
					createError={creation.error}
					isCreating={creation.isCreating}
					sessionId="session-1"
				/>
			);
		};
		render(<FailingCreate />);

		await userEvent.click(
			await screen.findByRole("button", { name: "Continue" }),
		);
		const id = await screen.findByRole("textbox", { name: /^ID/ });
		await userEvent.clear(id);
		await userEvent.type(id, "existing");
		await waitFor(() =>
			expect(
				screen.getByRole("combobox", { name: /^Organization/ }),
			).toHaveTextContent(MockOrganization.display_name),
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Create template" }),
		);

		const restoredId = await screen.findByRole("textbox", { name: /^ID/ });
		expect(restoredId).toHaveValue("existing");
		expect(restoredId).toBeInvalid();
		expect(restoredId).toHaveAccessibleDescription(message);
		expect(restoredId).toHaveFocus();

		await userEvent.type(restoredId, "-2");
		expect(restoredId).toBeValid();
	});
});
