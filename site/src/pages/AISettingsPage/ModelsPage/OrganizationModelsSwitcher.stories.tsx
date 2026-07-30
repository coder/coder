import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocation } from "react-router";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { OrganizationModelsSwitcher } from "./OrganizationModelsSwitcher";

const organizations = [MockDefaultOrganization, MockOrganization2];

// Probe rendered alongside the switcher so play functions can assert the
// in-memory story router's current path after a navigation.
const PathnameProbe = () => {
	const location = useLocation();
	return <div data-testid="pathname">{location.pathname}</div>;
};

const meta = {
	title: "pages/AISettingsPage/ModelsPage/OrganizationModelsSwitcher",
	component: OrganizationModelsSwitcher,
	decorators: [
		(Story) => (
			<>
				<Story />
				<PathnameProbe />
			</>
		),
	],
	args: {
		activeOrganization: MockDefaultOrganization,
		organizations,
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/organizations/${MockDefaultOrganization.name}/models`,
			},
			routing: [
				{
					path: "/ai/settings/organizations/:organization/models",
					useStoryElement: true,
				},
				{
					path: "/ai/settings/organizations/:organization/defaults",
					useStoryElement: true,
				},
			],
		}),
	},
} satisfies Meta<typeof OrganizationModelsSwitcher>;

export default meta;
type Story = StoryObj<typeof OrganizationModelsSwitcher>;

export const Closed: Story = {};

const switchOrganization = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const body = within(canvasElement.ownerDocument.body);

	await userEvent.click(
		canvas.getByRole("button", { name: /My Organization/i }),
	);
	const listbox = await body.findByRole("listbox");
	await waitFor(() => {
		expect(listbox).toBeVisible();
	});

	await userEvent.click(
		await body.findByRole("option", { name: /My Organization 2/i }),
	);
	// Selection closes the popover and navigates; the story router keeps the
	// same element mounted under the :organization param.
	await waitFor(() => {
		expect(body.queryByRole("listbox")).not.toBeInTheDocument();
	});
};

export const SwitchOrganization: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await switchOrganization(canvasElement);
		expect(canvas.getByTestId("pathname")).toHaveTextContent(
			`/ai/settings/organizations/${MockOrganization2.name}/models`,
		);
	},
};

export const SwitchOrganizationPreservesSubpage: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/organizations/${MockDefaultOrganization.name}/defaults`,
			},
			routing: [
				{
					path: "/ai/settings/organizations/:organization/models",
					useStoryElement: true,
				},
				{
					path: "/ai/settings/organizations/:organization/defaults",
					useStoryElement: true,
				},
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await switchOrganization(canvasElement);
		// The defaults subpage suffix survives the organization swap.
		expect(canvas.getByTestId("pathname")).toHaveTextContent(
			`/ai/settings/organizations/${MockOrganization2.name}/defaults`,
		);
	},
};
