import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { API } from "#/api/api";
import type { AuthorizationRequest } from "#/api/typesGenerated";
import {
	MockOrganization,
	MockOrganization2,
	MockTemplate,
	MockTemplateExample,
	MockTemplateVersion,
	MockTemplateVersionVariable1,
	MockTemplateVersionVariable2,
	MockTemplateVersionVariable3,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import CreateTemplatePage from "./CreateTemplatePage";

const renderPage = async (searchParams: URLSearchParams) => {
	// Render with the example ID so we don't need to upload a file
	const view = renderWithAuth(<CreateTemplatePage />, {
		route: `/templates/new?${searchParams.toString()}`,
		path: "/templates/new",
		// We need this because after creation, the user will be redirected to here
		extraRoutes: [
			{ path: "templates/:organization/:template/files", element: null },
			{ path: "templates/:template/files", element: null },
		],
	});
	// It is lazy loaded, so we have to wait for it to be rendered to not get an
	// act error
	await screen.findByLabelText("Icon", undefined, { timeout: 5000 });
	return view;
};

test("Create template from starter template", { timeout: 20_000 }, async () => {
	// Render page, fill the name and submit
	const searchParams = new URLSearchParams({
		exampleId: MockTemplateExample.id,
	});
	const { router, container } = await renderPage(searchParams);
	const form = container.querySelector("form") as HTMLFormElement;

	vi.spyOn(API, "createTemplateVersion").mockResolvedValueOnce({
		...MockTemplateVersion,
		job: {
			...MockTemplateVersion.job,
			status: "pending",
		},
	});
	vi.spyOn(API, "getTemplateVersion").mockResolvedValue({
		...MockTemplateVersion,
		job: {
			...MockTemplateVersion.job,
			status: "failed",
			error_code: "REQUIRED_TEMPLATE_VARIABLES",
		},
	});
	vi.spyOn(API, "getTemplateVersionVariables").mockResolvedValue([
		MockTemplateVersionVariable1,
		MockTemplateVersionVariable2,
		MockTemplateVersionVariable3,
	]);
	await userEvent.type(screen.getByLabelText(/Name/), "my-template");
	await userEvent.click(within(form).getByRole("button", { name: /save/i }));

	// Wait for the drawer error to be rendered
	await screen.findByRole("heading", { name: /missing variables/i });
	await userEvent.click(
		screen.getByRole("button", { name: /fill variables/i }),
	);

	// Wait for the variables form to be rendered and fill it
	await screen.findByText(/Variables/, undefined, { timeout: 5_000 });

	// Type first variable
	await userEvent.clear(screen.getByLabelText(/var.first_variable/));
	await userEvent.type(
		screen.getByLabelText(/var.first_variable/),
		"First value",
	);
	// Type second variable
	await userEvent.clear(screen.getByLabelText(/var.second_variable/));
	await userEvent.type(screen.getByLabelText(/var.second_variable/), "2");
	// Select third variable on radio
	await userEvent.click(screen.getByLabelText(/True/));
	// Setup the mock for the second template version creation before submit the form
	vi.clearAllMocks();
	vi.spyOn(API, "createTemplateVersion").mockResolvedValue(MockTemplateVersion);
	vi.spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion);
	vi.spyOn(API, "createTemplate").mockResolvedValue(MockTemplate);
	await userEvent.click(within(form).getByRole("button", { name: /save/i }));
	await waitFor(() => expect(API.createTemplate).toBeCalledTimes(1));
	expect(router.state.location.pathname).toEqual(
		`/templates/${MockTemplate.name}/files`,
	);
	expect(API.createTemplateVersion).toHaveBeenCalledWith("default", {
		example_id: "aws-windows",
		provisioner: "terraform",
		storage_method: "file",
		tags: {},
		user_variable_values: [
			{ name: "first_variable", value: "First value" },
			{ name: "second_variable", value: "2" },
			{ name: "third_variable", value: "true" },
		],
	});
});

test.each([false, true])(
	"Create template from duplicating a template (cross-organization: %s)",
	async (crossOrganization) => {
		const destination = crossOrganization ? MockOrganization2.name : "default";
		if (crossOrganization) {
			vi.spyOn(API, "getOrganizations").mockResolvedValue([
				MockOrganization,
				MockOrganization2,
			]);
			vi.spyOn(API, "getProvisionerDaemonsByOrganization").mockResolvedValue(
				[],
			);
			server.use(
				http.post<never, AuthorizationRequest>(
					"/api/v2/authcheck",
					async ({ request }) => {
						const { checks } = await request.json();
						return HttpResponse.json(
							Object.fromEntries(
								Object.keys(checks).map((name) => [
									name,
									name !== MockOrganization.id,
								]),
							),
						);
					},
				),
			);
		}
		vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([]);
		vi.spyOn(API, "getTemplateByName").mockResolvedValue(MockTemplate);
		vi.spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion);
		vi.spyOn(API, "getTemplateVersionVariables").mockResolvedValue([
			MockTemplateVersionVariable1,
		]);

		const searchParams = new URLSearchParams({
			fromTemplate: MockTemplate.id,
		});
		const { router } = await renderPage(searchParams);
		// Name and display name are using copy prefixes
		expect(screen.getByLabelText(/Name/)).toHaveValue(
			`${MockTemplate.name}-copy`,
		);
		expect(screen.getByLabelText(/Display name/)).toHaveValue(
			`Copy of ${MockTemplate.display_name}`,
		);
		// Variables are using the same values
		expect(
			screen.getByLabelText(MockTemplateVersionVariable1.description, {
				exact: false,
			}),
		).toHaveValue(MockTemplateVersionVariable1.value);
		// Create template
		vi.spyOn(API, "createTemplateVersion").mockResolvedValue(
			MockTemplateVersion,
		);
		vi.spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion);
		vi.spyOn(API, "createTemplate").mockResolvedValue(MockTemplate);
		await userEvent.click(screen.getByRole("button", { name: /save/i }));
		await waitFor(() => {
			expect(router.state.location.pathname).toEqual(
				crossOrganization
					? `/templates/${destination}/${MockTemplate.name}/files`
					: `/templates/${MockTemplate.name}/files`,
			);
		});
		expect(API.createTemplate).toHaveBeenCalledWith(
			destination,
			expect.objectContaining({ template_version_id: MockTemplateVersion.id }),
		);
	},
);

test("The page displays an error if the upload fails", async () => {
	const errMsg = "Unsupported content type header";
	vi.spyOn(API, "uploadFile").mockRejectedValueOnce(
		mockApiError({
			message: errMsg,
		}),
	);
	await renderPage(new URLSearchParams());

	const file = new File([""], "invalidfile.tar");
	const dropZone = screen.getByTestId("drop-zone");

	fireEvent.drop(dropZone, {
		dataTransfer: { files: [file] },
	});

	await waitFor(() => expect(API.uploadFile).toHaveBeenCalledTimes(1));

	// Error toast should be displayed
	expect(await screen.findByText(errMsg)).toBeInTheDocument();

	// The upload box should have reset itself
	expect(
		await screen.findByText(/The template has to be a .tar or .zip file/),
	).toBeInTheDocument();
});
