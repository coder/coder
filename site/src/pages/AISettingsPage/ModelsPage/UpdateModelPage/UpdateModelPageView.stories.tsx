import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocation } from "react-router";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { chatModel, chatModels } from "#/api/queries/chats";
import {
	MockDefaultOrganization,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import { OrganizationModelsContext } from "../organizationModels";
import {
	MockAnthropicProviderState,
	MockOpenAIProviderState,
	mockGPT5,
} from "../testFixtures";
import UpdateModelPage from "./UpdateModelPage";
import UpdateModelPageView from "./UpdateModelPageView";

const LocationProbe = () => {
	const location = useLocation();
	return (
		<span role="status" aria-label="Current location">
			{location.pathname}
			{location.search}
		</span>
	);
};

const meta: Meta<typeof UpdateModelPageView> = {
	title: "pages/AISettingsPage/ModelsPage/UpdateModelPageView",
	component: UpdateModelPageView,
	decorators: [
		(Story) => (
			<OrganizationModelsContext.Provider
				value={{
					organization: MockDefaultOrganization,
					accessibleOrganizations: [MockDefaultOrganization],
					permissions: MockOrganizationPermissions,
					requestedOrganizationDenied: false,
				}}
			>
				<Story />
			</OrganizationModelsContext.Provider>
		),
		withToaster,
	],
	args: {
		state: "loaded",
		model: mockGPT5,
		providerStates: [MockOpenAIProviderState, MockAnthropicProviderState],
		selectedProviderState: MockOpenAIProviderState,
		onProviderChange: fn(),
		isSaving: false,
		isDeleting: false,
		canCreateModel: true,
		canUpdateModel: true,
		canDeleteModel: true,
		onUpdateModel: fn(async () => undefined),
		onDeleteModel: fn(async () => undefined),
		onDuplicate: fn(),
		onToggleEnabled: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof UpdateModelPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: /^update model$/i }),
		).toBeVisible();
		await expect(canvas.getByLabelText(/model identifier/i)).toBeEnabled();
		// The organization is informational on the edit page: switching org
		// while editing would 404 the model, so it renders as a static value
		// rather than a picker.
		await expect(
			canvas.getByLabelText(
				`Organization ${MockDefaultOrganization.display_name}`,
			),
		).toBeVisible();
		expect(
			canvas.queryByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		).not.toBeInTheDocument();
	},
};

export const CreateOnlyUser: Story = {
	args: {
		canCreateModel: true,
		canUpdateModel: false,
		canDeleteModel: false,
	},
	play: async ({ canvasElement, args }) => {
		if (args.state !== "loaded") {
			throw new Error("Expected the loaded model state.");
		}
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText(/model identifier/i)).toBeDisabled();
		await expect(
			canvas.queryByRole("button", { name: /^update model$/i }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("switch", { name: /model enabled/i }),
		).not.toBeInTheDocument();
		await userEvent.click(
			canvas.getByRole("button", { name: /model actions/i }),
		);
		const menu = await screen.findByRole("menu");
		await expect(
			within(menu).queryByRole("menuitem", { name: /delete/i }),
		).not.toBeInTheDocument();
		await userEvent.click(
			within(menu).getByRole("menuitem", { name: /duplicate model/i }),
		);
		await expect(args.onDuplicate).toHaveBeenCalledTimes(1);
	},
};

export const UpdateOnlyUser: Story = {
	args: {
		model: { ...mockGPT5, is_default: false },
		canCreateModel: false,
		canUpdateModel: true,
		canDeleteModel: false,
	},
	play: async ({ canvasElement, args }) => {
		if (args.state !== "loaded") {
			throw new Error("Expected the loaded model state.");
		}
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText(/model identifier/i)).toBeEnabled();
		await userEvent.click(
			canvas.getByRole("switch", { name: /model enabled/i }),
		);
		await expect(args.onToggleEnabled).toHaveBeenCalledTimes(1);
		await userEvent.clear(canvas.getByLabelText(/display name/i));
		await userEvent.type(
			canvas.getByLabelText(/display name/i),
			"Updated model",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: /^update model$/i }),
		);
		await expect(args.onUpdateModel).toHaveBeenCalledTimes(1);
		await expect(
			canvas.queryByRole("button", { name: /model actions/i }),
		).not.toBeInTheDocument();
	},
};

export const DeleteOnlyUser: Story = {
	args: {
		canCreateModel: false,
		canUpdateModel: false,
		canDeleteModel: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText(/model identifier/i)).toBeDisabled();
		await userEvent.click(
			canvas.getByRole("button", { name: /model actions/i }),
		);
		const menu = await screen.findByRole("menu");
		await expect(
			within(menu).queryByRole("menuitem", { name: /duplicate model/i }),
		).not.toBeInTheDocument();
		await userEvent.click(
			within(menu).getByRole("menuitem", { name: /delete/i }),
		);
		await expect(await screen.findByRole("dialog")).toHaveAttribute(
			"data-state",
			"open",
		);
	},
};

export const ReadOnlyUser: Story = {
	args: {
		canCreateModel: false,
		canUpdateModel: false,
		canDeleteModel: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText(/model identifier/i)).toBeDisabled();
		await expect(
			canvas.queryByRole("button", { name: /^update model$/i }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("switch", { name: /model enabled/i }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("button", { name: /model actions/i }),
		).not.toBeInTheDocument();
	},
};

export const DuplicateNavigatesToStructuralAddPath: Story = {
	render: () => (
		<>
			<UpdateModelPage />
			<LocationProbe />
		</>
	),
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/models/:modelId",
				pathParams: { modelId: mockGPT5.id },
				searchParams: { org: MockDefaultOrganization.name },
			},
			routing: [
				{
					path: "/ai/settings/models/:modelId",
					useStoryElement: true,
				},
				{ path: "/ai/settings/models/add", element: <LocationProbe /> },
			],
		}),
		queries: [
			{
				key: chatModel(MockDefaultOrganization.id, mockGPT5.id).queryKey,
				data: mockGPT5,
			},
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: {
					models: [mockGPT5],
					providers: [MockOpenAIProviderState.providerDescriptor],
					unsupported_providers: [],
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /model actions/i }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: /duplicate model/i }),
		);

		await waitFor(() =>
			expect(
				canvas.getByRole("status", { name: "Current location" }),
			).toHaveTextContent(
				`/ai/settings/models/add?provider=${MockOpenAIProviderState.key}&duplicate=${mockGPT5.id}&org=${MockDefaultOrganization.name}`,
			),
		);
	},
};

export const RefetchError: Story = {
	args: { refetchError: new Error("Failed to refresh model") },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Failed to refresh model")).toBeVisible();
		expect(canvas.getByLabelText(/model identifier/i)).toBeEnabled();
	},
};

export const LoadError: Story = {
	render: () => (
		<UpdateModelPageView
			state="error"
			error={new Error("Failed to load model")}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Failed to load model")).toBeVisible();
	},
};

export const ModelNotFound: Story = {
	render: () => <UpdateModelPageView state="notFound" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("This page could not be found."),
		).toBeVisible();
	},
};
