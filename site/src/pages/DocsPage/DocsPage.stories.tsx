import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import DocsPage from "./DocsPage";
import type { DocsManifest } from "./docsContent";

const MockDocsManifest: DocsManifest = {
	versions: ["main"],
	routes: [
		{
			title: "About",
			path: "./README.md",
			children: [{ title: "Quickstart", path: "./tutorials/quickstart.md" }],
		},
		{
			title: "Install",
			path: "./install/index.md",
			children: [{ title: "Airgap", path: "./install/airgap.md" }],
		},
	],
};

const MockReadme = `# Welcome to Coder

Some **markdown** content with a [relative link](./install/index.md)
and a callout:

> [!NOTE]
> This renders as an alert.
`;

const MockInstall = `# Install

Install instructions with an image:

![diagram](./images/diagram.png)
`;

const meta: Meta<typeof DocsPage> = {
	title: "pages/DocsPage",
	component: DocsPage,
	parameters: {
		layout: "fullscreen",
		queries: [
			{ key: ["docs", "manifest"], data: MockDocsManifest },
			{ key: ["docs", "page", "README.md"], data: MockReadme },
			{ key: ["docs", "page", "install/index.md"], data: MockInstall },
		],
		reactRouter: reactRouterParameters({
			location: { path: "/docs" },
			routing: { path: "/docs/*" },
		}),
	},
};

export default meta;
type Story = StoryObj<typeof DocsPage>;

export const Home: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(
				canvas.getByRole("heading", { name: "Welcome to Coder" }),
			).toBeVisible(),
		);
		await expect(
			canvas.getByRole("link", { name: "relative link" }),
		).toHaveAttribute("href", "/docs/install");
	},
};

export const NavigateViaSidebar: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("link", { name: "Install" }));
		await waitFor(() =>
			expect(canvas.getByRole("heading", { name: "Install" })).toBeVisible(),
		);
	},
};

export const PageNotFound: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/docs/does/not/exist" },
			routing: { path: "/docs/*" },
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(
				canvas.getByRole("heading", { name: "Page not found" }),
			).toBeVisible(),
		);
	},
};

export const ImageRewrite: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/docs/install" },
			routing: { path: "/docs/*" },
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			const image = canvas.getByRole("img", { name: "diagram" });
			expect(image.getAttribute("src")).toMatch(
				/^https:\/\/raw\.githubusercontent\.com\/coder\/coder\/.+\/docs\/install\/images\/diagram\.png$/,
			);
		});
	},
};

export const SidebarToggle: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("link", { name: "Airgap" })).toBeNull();
		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle Install section" }),
		);
		await expect(canvas.getByRole("link", { name: "Airgap" })).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle Install section" }),
		);
		await waitFor(() =>
			expect(canvas.queryByRole("link", { name: "Airgap" })).toBeNull(),
		);
	},
};
