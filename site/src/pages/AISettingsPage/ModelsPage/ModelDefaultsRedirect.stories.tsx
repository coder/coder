import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocation } from "react-router";
import { expect, screen } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { ModelDefaultsRedirect } from "./ModelDefaultsRedirect";

const LocationProbe = () => {
	const location = useLocation();
	return (
		<p role="status" aria-label="Current location">
			{location.pathname}
			{location.search}
		</p>
	);
};

const meta: Meta<typeof ModelDefaultsRedirect> = {
	title: "pages/AISettingsPage/ModelDefaultsRedirect",
	component: ModelDefaultsRedirect,
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/models/defaults",
				searchParams: { org: "engineering", source: "bookmark" },
			},
			routing: [
				{
					path: "/ai/settings/models/defaults",
					useStoryElement: true,
				},
				{
					path: "/ai/settings/coder-agents",
					element: <LocationProbe />,
				},
			],
		}),
	},
};
export default meta;
type Story = StoryObj<typeof ModelDefaultsRedirect>;

export const PreservesQueryParameters: Story = {
	play: async () => {
		await expect(
			await screen.findByRole("status", { name: "Current location" }),
		).toHaveTextContent(
			"/ai/settings/coder-agents?org=engineering&source=bookmark",
		);
	},
};
