import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocation, useNavigate } from "react-router";
import { expect, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { TemplateRedirectController } from "./TemplateRedirectController";

const Destination = () => {
	const location = useLocation();
	return (
		<output aria-label="Destination">
			{location.pathname}
			{location.search}
			{location.hash}
		</output>
	);
};
const meta = {
	title: "pages/TemplatePage/TemplateRedirectController",
	component: TemplateRedirectController,
	render: function Render() {
		const navigate = useNavigate();
		const location = useLocation();
		return location.hash ? (
			<TemplateRedirectController />
		) : (
			<button
				type="button"
				onClick={() => {
					navigate({
						search: "?tab=ACCESS.md",
						hash: "#remote-desktop-access",
					});
				}}
			>
				Open heading link
			</button>
		);
	},
	decorators: [withDashboardProvider],
} satisfies Meta<typeof TemplateRedirectController>;
export default meta;
type Story = StoryObj<typeof meta>;

export const RemoveOrganization: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: `/templates/${MockDefaultOrganization.name}/windows/docs`,
			},
			routing: [
				{
					path: "/templates/:organization/:template/docs",
					useStoryElement: true,
				},
				{ path: "/templates/:template/docs", element: <Destination /> },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Open heading link" }),
		);
		await expect(
			await within(canvasElement).findByRole("status", { name: "Destination" }),
		).toHaveTextContent(
			"/templates/windows/docs?tab=ACCESS.md#remote-desktop-access",
		);
	},
};
export const AddOrganization: Story = {
	parameters: {
		showOrganizations: true,
		reactRouter: reactRouterParameters({
			location: {
				path: "/templates/windows/docs",
			},
			routing: [
				{ path: "/templates/:template/docs", useStoryElement: true },
				{
					path: "/templates/:organization/:template/docs",
					element: <Destination />,
				},
			],
		}),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Open heading link" }),
		);
		await expect(
			await within(canvasElement).findByRole("status", { name: "Destination" }),
		).toHaveTextContent(
			`/templates/${MockDefaultOrganization.name}/windows/docs?tab=ACCESS.md#remote-desktop-access`,
		);
	},
};
