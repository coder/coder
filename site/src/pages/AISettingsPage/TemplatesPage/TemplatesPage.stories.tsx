import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { organizationsPermissions } from "#/api/queries/organizations";
import { getTemplatesQueryKey } from "#/api/queries/templates";
import type { Template } from "#/api/typesGenerated";
import { createDeferred, type Deferred } from "#/testHelpers/deferred";
import {
	MockDefaultOrganization,
	MockNoOrganizationPermissions,
	MockOrganization2,
	MockOrganizationPermissions,
	MockTemplate,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import TemplatesPage from "./TemplatesPage";

const mockSecondTemplate: Template = {
	...MockTemplate,
	id: "second-template",
	name: "second-template",
	display_name: "Second Template",
};

const mockUnauthorizedTemplate: Template = {
	...MockTemplate,
	id: "unauthorized-template",
	name: "unauthorized-template",
	display_name: "Unauthorized Template",
	organization_id: MockOrganization2.id,
	organization_name: MockOrganization2.name,
	organization_display_name: MockOrganization2.display_name,
};

type ToggleDeferreds = {
	first: Deferred<Template | null>;
	second: Deferred<Template | null>;
	retry: Deferred<Template | null>;
};

let toggleDeferreds: ToggleDeferreds | undefined;
let refetchedTemplates: Template[] = [];

const meta = {
	title: "pages/AISettingsPage/TemplatesPage/TemplatesPage",
	component: TemplatesPage,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: {
			updateAnyTemplate: true,
		},
		queries: [
			{
				key: getTemplatesQueryKey({ q: "" }),
				data: [MockTemplate],
			},
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
		],
	},
} satisfies Meta<typeof TemplatesPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ServerSideFilter: Story = {
	parameters: {
		queries: [
			{
				key: getTemplatesQueryKey({ q: "" }),
				data: [MockTemplate, mockSecondTemplate],
			},
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplates").mockImplementation((options) => {
			const query = options && "q" in options ? options.q : "";
			return Promise.resolve(
				query === "Second"
					? [mockSecondTemplate]
					: [MockTemplate, mockSecondTemplate],
			);
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		expect(await canvas.findByText("Test Template")).toBeVisible();
		expect(canvas.getByText("Second Template")).toBeVisible();

		await user.type(canvas.getByRole("textbox", { name: "Filter" }), "Second");

		await waitFor(() =>
			expect(API.getTemplates).toHaveBeenCalledWith({ q: "Second" }),
		);
		expect(await canvas.findByText("Second Template")).toBeVisible();
		expect(canvas.queryByText("Test Template")).not.toBeInTheDocument();
	},
};

export const ConcurrentToggles: Story = {
	parameters: {
		queries: [
			{
				key: getTemplatesQueryKey({ q: "" }),
				data: [MockTemplate, mockSecondTemplate],
			},
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
		],
	},
	beforeEach: () => {
		toggleDeferreds = {
			first: createDeferred<Template | null>(),
			second: createDeferred<Template | null>(),
			retry: createDeferred<Template | null>(),
		};
		refetchedTemplates = [MockTemplate, mockSecondTemplate];
		let mutationCall = 0;
		spyOn(API, "updateTemplateMeta").mockImplementation(() => {
			mutationCall += 1;
			if (!toggleDeferreds) {
				throw new Error("Toggle deferreds were not initialized.");
			}
			switch (mutationCall) {
				case 1:
					return toggleDeferreds.first.promise;
				case 2:
					return toggleDeferreds.second.promise;
				case 3:
					return toggleDeferreds.retry.promise;
				default:
					throw new Error(`Unexpected mutation call ${mutationCall}.`);
			}
		});
		spyOn(API, "getTemplates").mockImplementation((options) => {
			const query = options && "q" in options ? options.q : "";
			return Promise.resolve(
				query === "Second"
					? refetchedTemplates.filter(
							(template) => template.id === mockSecondTemplate.id,
						)
					: refetchedTemplates,
			);
		});
	},
	play: async ({ canvasElement }) => {
		const deferreds = toggleDeferreds;
		if (!deferreds) {
			throw new Error("Toggle deferreds were not initialized.");
		}
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();
		const firstSwitch = await canvas.findByRole("switch", {
			name: "Allow Coder Agents to create workspaces using Test Template in My Organization",
		});
		const secondSwitch = canvas.getByRole("switch", {
			name: "Allow Coder Agents to create workspaces using Second Template in My Organization",
		});
		await user.click(firstSwitch);
		await user.click(secondSwitch);
		await waitFor(() => expect(firstSwitch).toBeDisabled());
		await waitFor(() => expect(secondSwitch).toBeDisabled());
		expect(API.updateTemplateMeta).toHaveBeenNthCalledWith(1, MockTemplate.id, {
			agents_allowed: false,
		});
		expect(API.updateTemplateMeta).toHaveBeenNthCalledWith(
			2,
			mockSecondTemplate.id,
			{ agents_allowed: false },
		);

		refetchedTemplates = [
			MockTemplate,
			{ ...mockSecondTemplate, agents_allowed: false },
		];
		deferreds.first.reject(
			mockApiError({ message: "Template access is locked." }),
		);
		deferreds.second.resolve({ ...mockSecondTemplate, agents_allowed: false });

		const errorToast = await body.findByText(
			"Test Template in My Organization: Template access is locked.",
		);
		await waitFor(() => expect(errorToast).toBeVisible());
		await waitFor(() => expect(firstSwitch).toBeEnabled());
		await waitFor(() => expect(secondSwitch).toBeEnabled());
		expect(firstSwitch).toBeChecked();
		expect(secondSwitch).not.toBeChecked();

		const filter = canvas.getByRole("textbox", { name: "Filter" });
		await user.type(filter, "Second");
		await waitFor(() =>
			expect(API.getTemplates).toHaveBeenCalledWith({ q: "Second" }),
		);
		expect(await canvas.findByText("Second Template")).toBeVisible();
		expect(
			canvas.queryByRole("switch", {
				name: "Allow Coder Agents to create workspaces using Test Template in My Organization",
			}),
		).not.toBeInTheDocument();
		const filteredErrorToast = await body.findByText(
			"Test Template in My Organization: Template access is locked.",
		);
		await waitFor(() => expect(filteredErrorToast).toBeVisible());

		await user.clear(filter);
		const retrySwitch = await canvas.findByRole("switch", {
			name: "Allow Coder Agents to create workspaces using Test Template in My Organization",
		});
		await user.click(retrySwitch);
		await waitFor(() => expect(retrySwitch).toBeDisabled());
		expect(API.updateTemplateMeta).toHaveBeenNthCalledWith(3, MockTemplate.id, {
			agents_allowed: false,
		});

		refetchedTemplates = [
			{ ...MockTemplate, agents_allowed: false },
			{ ...mockSecondTemplate, agents_allowed: false },
		];
		deferreds.retry.resolve({ ...MockTemplate, agents_allowed: false });
		await waitFor(() => expect(retrySwitch).toBeEnabled());
		await waitFor(() => expect(retrySwitch).not.toBeChecked());
	},
};

export const DisplaysFallbackMutationError: Story = {
	beforeEach: () => {
		spyOn(API, "updateTemplateMeta").mockRejectedValue({});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const templateSwitch = await canvas.findByRole("switch", {
			name: "Allow Coder Agents to create workspaces using Test Template in My Organization",
		});

		await userEvent.click(templateSwitch);

		const errorToast = await body.findByText(
			"Test Template in My Organization: Failed to update whether Coder Agents can create workspaces.",
		);
		await waitFor(() => expect(errorToast).toBeVisible());
		expect(API.updateTemplateMeta).toHaveBeenCalledWith(MockTemplate.id, {
			agents_allowed: false,
		});
		await waitFor(() => expect(templateSwitch).toBeEnabled());
		expect(templateSwitch).toBeChecked();
	},
};

export const OrganizationTemplateAdmin: Story = {
	parameters: {
		permissions: {
			updateAnyTemplate: true,
			viewAllUsers: true,
		},
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: organizationsPermissions([
					MockDefaultOrganization.id,
					MockOrganization2.id,
				]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
					[MockOrganization2.id]: MockNoOrganizationPermissions,
				},
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([
			MockTemplate,
			mockUnauthorizedTemplate,
		]);
		spyOn(API, "getUsers").mockResolvedValue({ users: [], count: 0 });
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Test Template")).toBeVisible();
		expect(canvas.queryByText("Unauthorized Template")).not.toBeInTheDocument();
		await waitFor(() => expect(API.getTemplates).toHaveBeenCalled());
	},
};

export const PermissionsRetryRecovers: Story = {
	parameters: {
		queries: [],
	},
	beforeEach: () => {
		const checkAuthorization = spyOn(API, "checkAuthorization");
		checkAuthorization
			.mockRejectedValueOnce(
				mockApiError({ message: "Failed to load organization permissions." }),
			)
			.mockImplementation(async ({ checks }) =>
				Object.fromEntries(
					Object.keys(checks).map((key) => [
						key,
						key.endsWith(".updateTemplates"),
					]),
				),
			);
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Failed to load templates.")).toBeVisible();
		expect(API.getTemplates).not.toHaveBeenCalled();

		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));

		expect(await canvas.findByText("Test Template")).toBeVisible();
		await waitFor(() =>
			expect(API.checkAuthorization).toHaveBeenCalledTimes(2),
		);
		await waitFor(() => expect(API.getTemplates).toHaveBeenCalled());
	},
};

export const FetchesWhenAllowed: Story = {
	parameters: {
		permissions: {
			updateAnyTemplate: true,
			viewAllUsers: true,
		},
		queries: [
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
		spyOn(API, "getUsers").mockResolvedValue({
			users: [MockUserOwner],
			count: 1,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Test Template")).toBeVisible();
		await waitFor(() => expect(API.getTemplates).toHaveBeenCalled());
		await waitFor(() => expect(API.getUsers).toHaveBeenCalled());
	},
};

export const NoUpdateTemplatesPermission: Story = {
	parameters: {
		permissions: {
			updateAnyTemplate: false,
		},
	},
	play: async () => {
		const body = within(document.body);
		expect(
			await body.findByText("You don't have permission to view this page"),
		).toBeInTheDocument();
		expect(body.queryByText("Test Template")).not.toBeInTheDocument();
	},
};
