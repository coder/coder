import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocation } from "react-router";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { chatModels } from "#/api/queries/chats";
import { organizationsPermissions } from "#/api/queries/organizations";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import AddModelPage from "./AddModelPage/AddModelPage";
import OrganizationModelsLayout from "./OrganizationModelsLayout";

const LocationProbe = () => {
	const location = useLocation();
	return (
		<div data-testid="location-probe">
			{location.pathname}
			{location.search}
		</div>
	);
};

const meta: Meta<typeof OrganizationModelsLayout> = {
	title: "pages/AISettingsPage/OrganizationModelsLayout",
	component: OrganizationModelsLayout,
	decorators: [withDashboardProvider],
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/models",
				searchParams: { org: MockDefaultOrganization.name },
			},
			routing: [{ path: "*", useStoryElement: true }],
		}),
		queries: [
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
			{
				key: chatModels(MockOrganization2.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
			{
				key: organizationsPermissions([
					MockDefaultOrganization.id,
					MockOrganization2.id,
				]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
					[MockOrganization2.id]: MockOrganizationPermissions,
				},
			},
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
			{
				key: organizationsPermissions([MockOrganization2.id]).queryKey,
				data: { [MockOrganization2.id]: MockOrganizationPermissions },
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationModelsLayout>;

export const SwitchOrganizationPreservesAuxiliaryParameters: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/models/add",
				searchParams: {
					org: MockDefaultOrganization.name,
					provider: "openai",
					duplicate: "model-id",
				},
			},
			routing: [{ path: "*", useStoryElement: true }],
		}),
	},
	render: () => (
		<>
			<OrganizationModelsLayout />
			<LocationProbe />
		</>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", {
				name: new RegExp(MockDefaultOrganization.display_name, "i"),
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: new RegExp(MockOrganization2.display_name),
			}),
		);
		await waitFor(() => {
			expect(screen.getByTestId("location-probe")).toHaveTextContent(
				`/ai/settings/models/add?org=${MockOrganization2.name}&provider=openai&duplicate=model-id`,
			);
		});
	},
};

export const InvalidRequestedOrganizationFallsBackToDefault: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/models",
				searchParams: { org: "missing" },
			},
			routing: [{ path: "*", useStoryElement: true }],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("button", {
				name: new RegExp(MockDefaultOrganization.display_name, "i"),
			}),
		).toBeVisible();
	},
};

export const InvalidRequestedOrganizationDeniesAdd: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "createChatModel");
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/models/add",
				searchParams: { org: "missing" },
			},
			routing: [
				{
					path: "/ai/settings/models",
					useStoryElement: true,
					children: [{ path: "add", element: <AddModelPage /> }],
				},
			],
		}),
		queries: [
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
			{
				key: chatModels(MockOrganization2.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
			{
				key: organizationsPermissions([
					MockDefaultOrganization.id,
					MockOrganization2.id,
				]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
					[MockOrganization2.id]: MockOrganizationPermissions,
				},
			},
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() =>
			expect(
				body.getByRole("heading", {
					name: "You don't have permission to view this page",
				}),
			).toBeVisible(),
		);
		expect(
			body.queryByRole("heading", { name: /add an? .* model/i }),
		).not.toBeInTheDocument();
		expect(API.experimental.createChatModel).not.toHaveBeenCalled();
	},
};

export const SingleAccessibleOrganizationHidesPicker: Story = {
	parameters: {
		organizations: [MockDefaultOrganization],
		queries: [
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(canvas.queryByTestId("organization-autocomplete")).toBeNull(),
		);
	},
};

const duplicateNameOrganization = {
	...MockOrganization2,
	display_name: MockDefaultOrganization.display_name,
};

export const DuplicateDisplayNamesAreDisambiguated: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, duplicateNameOrganization],
		queries: [
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
			{
				key: chatModels(duplicateNameOrganization.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
			{
				key: organizationsPermissions([
					MockDefaultOrganization.id,
					duplicateNameOrganization.id,
				]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
					[duplicateNameOrganization.id]: MockOrganizationPermissions,
				},
			},
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", {
				name: new RegExp(MockDefaultOrganization.name),
			}),
		);
		expect(
			await screen.findByRole("option", {
				name: new RegExp(duplicateNameOrganization.name),
			}),
		).toHaveTextContent(duplicateNameOrganization.name);
	},
};

export const PermissionLoadingShowsLoader: Story = {
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockReturnValue(
			new Promise(() => undefined),
		);
	},
	parameters: {
		queries: [
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
			{
				key: chatModels(MockOrganization2.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByRole("status")).toBeVisible();
		expect(
			canvas.queryByText("You don't have permission to view this page"),
		).not.toBeInTheDocument();
	},
};

export const PermissionLoadErrorShowsAlert: Story = {
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockRejectedValue(
			new Error("Failed to load organization permissions"),
		);
	},
	parameters: {
		queries: [
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
			{
				key: chatModels(MockOrganization2.id).queryKey,
				data: { models: [], providers: [], unsupported_providers: [] },
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to load organization permissions"),
		).toBeVisible();
	},
};

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatModels").mockImplementation(
			() => new Promise(() => undefined),
		);
	},
	parameters: { queries: [] },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByRole("status")).toBeVisible();
	},
};

export const NoReadableOrganizationIsNotFound: Story = {
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockResolvedValue({});
		spyOn(API.experimental, "getChatModels").mockRejectedValue({
			isAxiosError: true,
			response: { status: 403 },
		});
	},
	parameters: { queries: [] },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("This page could not be found."),
		).toBeVisible();
	},
};
