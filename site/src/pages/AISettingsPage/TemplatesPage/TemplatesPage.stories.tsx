import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { getTemplatesQueryKey } from "#/api/queries/templates";
import type { Template } from "#/api/typesGenerated";
import { createDeferred, type Deferred } from "#/testHelpers/deferred";
import { MockTemplate, MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import TemplatesPage from "./TemplatesPage";

const secondTemplate: Template = {
	...MockTemplate,
	id: "second-template",
	name: "second-template",
	display_name: "Second Template",
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
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: {
			editDeploymentConfig: true,
			updateTemplates: true,
		},
		queries: [
			{
				key: getTemplatesQueryKey({ q: "" }),
				data: [MockTemplate],
			},
		],
	},
} satisfies Meta<typeof TemplatesPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const HasBothPermissions: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Test Template")).toBeVisible();
	},
};

export const ServerSideFilter: Story = {
	parameters: {
		queries: [
			{
				key: getTemplatesQueryKey({ q: "" }),
				data: [MockTemplate, secondTemplate],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplates").mockImplementation((options) => {
			const query = options && "q" in options ? options.q : "";
			return Promise.resolve(
				query === "Second" ? [secondTemplate] : [MockTemplate, secondTemplate],
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
				data: [MockTemplate, secondTemplate],
			},
		],
	},
	beforeEach: () => {
		toggleDeferreds = {
			first: createDeferred<Template | null>(),
			second: createDeferred<Template | null>(),
			retry: createDeferred<Template | null>(),
		};
		refetchedTemplates = [MockTemplate, secondTemplate];
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
		spyOn(API, "getTemplates").mockImplementation(() =>
			Promise.resolve(refetchedTemplates),
		);
	},
	play: async ({ canvasElement }) => {
		const deferreds = toggleDeferreds;
		if (!deferreds) {
			throw new Error("Toggle deferreds were not initialized.");
		}
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const firstSwitch = await canvas.findByRole("switch", {
			name: "Allow Coder Agents to create workspaces with Test Template in My Organization",
		});
		const secondSwitch = canvas.getByRole("switch", {
			name: "Allow Coder Agents to create workspaces with Second Template in My Organization",
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
			secondTemplate.id,
			{ agents_allowed: false },
		);

		refetchedTemplates = [
			MockTemplate,
			{ ...secondTemplate, agents_allowed: false },
		];
		deferreds.first.reject(new Error("Template access is locked."));
		deferreds.second.resolve({ ...secondTemplate, agents_allowed: false });

		const alert = await canvas.findByRole("alert");
		expect(alert).toHaveTextContent(
			"Test Template: Template access is locked.",
		);
		await waitFor(() => expect(firstSwitch).toBeEnabled());
		await waitFor(() => expect(secondSwitch).toBeEnabled());
		expect(firstSwitch).toBeChecked();
		expect(secondSwitch).not.toBeChecked();

		await user.click(firstSwitch);
		await waitFor(() => expect(firstSwitch).toBeDisabled());
		expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
		expect(API.updateTemplateMeta).toHaveBeenNthCalledWith(3, MockTemplate.id, {
			agents_allowed: false,
		});

		refetchedTemplates = [
			{ ...MockTemplate, agents_allowed: false },
			{ ...secondTemplate, agents_allowed: false },
		];
		deferreds.retry.resolve({ ...MockTemplate, agents_allowed: false });
		await waitFor(() => expect(firstSwitch).toBeEnabled());
		await waitFor(() => expect(firstSwitch).not.toBeChecked());
	},
};

export const NoDeploymentConfigPermission: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			updateTemplates: true,
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

export const NoUpdateTemplatesPermission: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: true,
			updateTemplates: false,
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
