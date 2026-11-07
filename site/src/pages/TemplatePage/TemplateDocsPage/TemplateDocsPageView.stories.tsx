import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { useLocation } from "react-router";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { TemplateDocsPageView } from "./TemplateDocsPageView";

const meta = {
	title: "pages/TemplatePage/TemplateDocsPageView",
	component: TemplateDocsPageView,
	decorators: [
		(Story) => (
			<>
				<header className="sticky top-0 z-40 flex h-[72px] items-center bg-surface-primary px-6">
					Navigation
				</header>
				<Story />
			</>
		),
	],
	args: {
		files: {
			"README.md":
				"# Windows workspace\n\n" +
				"Setup instructions.\n\n".repeat(25) +
				"## Remote Desktop Access\n\nConnect to Windows.\n\n## Remote Desktop Access\n\nAnother connection.\n\n" +
				"Further instructions.\n\n".repeat(25),
			"ACCESS.md": "# Access\n\n## SSH\n\nConnect using SSH.",
		},
	},
} satisfies Meta<typeof TemplateDocsPageView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Documents: Story = {
	render: function Render(args) {
		const location = useLocation();
		return (
			<>
				<output aria-label="Current document URL">
					{location.search}
					{location.hash}
				</output>
				<TemplateDocsPageView {...args} />
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const remote = canvas.getAllByRole("heading", {
			name: /Remote Desktop Access/,
		});
		await expect(remote[0]).toHaveAttribute("id", "remote-desktop-access");
		await expect(remote[1]).toHaveAttribute("id", "remote-desktop-access-1");
		await userEvent.hover(remote[0]);
		const permalink = within(remote[0]).getByRole("link", {
			name: "Link to remote-desktop-access",
		});
		await expect(permalink).toBeVisible();
		await userEvent.click(permalink);
		await expect(
			canvas.getByRole("status", { name: "Current document URL" }),
		).toHaveTextContent("#remote-desktop-access");
		await userEvent.click(canvas.getByRole("tab", { name: "ACCESS.md" }));
		await expect(canvas.getByRole("heading", { name: /SSH/ })).toBeVisible();
		await expect(
			canvas.queryByRole("heading", { name: /Windows workspace/ }),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("status", { name: "Current document URL" }),
		).toHaveTextContent("?tab=ACCESS.md");
		await userEvent.click(
			canvas.getByRole("button", { name: "Table of contents" }),
		);
		const toc = within(document.body).getByRole("navigation", {
			name: "Table of contents",
		});
		await userEvent.click(within(toc).getByRole("link", { name: "SSH" }));
		await expect(
			canvas.getByRole("status", { name: "Current document URL" }),
		).toHaveTextContent("?tab=ACCESS.md#ssh");
		await expect(canvas.getByRole("heading", { name: /SSH/ })).toHaveFocus();
	},
};

export const DelayedHeadingLink: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { hash: "#remote-desktop-access" },
		}),
	},
	beforeEach: () => {
		const scroll = spyOn(Element.prototype, "scrollIntoView");
		return () => scroll.mockRestore();
	},
	render: function Render(args) {
		const [loaded, setLoaded] = useState(false);
		return loaded ? (
			<TemplateDocsPageView {...args} />
		) : (
			<button type="button" onClick={() => setLoaded(true)}>
				Finish loading README
			</button>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByRole("heading")).not.toBeInTheDocument();
		await userEvent.click(
			canvas.getByRole("button", { name: "Finish loading README" }),
		);
		await expect(
			canvas.getAllByRole("heading", { name: /Remote Desktop Access/ })[0],
		).toHaveAttribute("id", "remote-desktop-access");
		await expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
	},
};

export const Empty: Story = {
	args: { files: {} },
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(
				"This template has no Markdown documentation.",
			),
		).toBeVisible();
	},
};
export const MissingDocument: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { searchParams: { tab: "missing.md" } },
		}),
	},
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(
				"Document not found. Select a document above.",
			),
		).toBeVisible();
		await userEvent.click(
			within(canvasElement).getByRole("tab", { name: "README.md" }),
		);
		await expect(
			within(canvasElement).getByRole("heading", { name: /Windows workspace/ }),
		).toBeVisible();
	},
};

export const KeyboardTabs: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const readme = canvas.getByRole("tab", { name: "README.md" });
		readme.focus();
		await userEvent.keyboard("{ArrowRight}{Enter}");
		await expect(
			canvas.getByRole("tab", { name: "ACCESS.md" }),
		).toHaveAttribute("aria-selected", "true");
		await expect(canvas.getByRole("heading", { name: /SSH/ })).toBeVisible();
	},
};

export const DirectDocumentHeading: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { searchParams: { tab: "ACCESS.md" }, hash: "#ssh" },
		}),
	},
	beforeEach: DelayedHeadingLink.beforeEach,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("heading", { name: /SSH/ })).toHaveAttribute(
			"id",
			"ssh",
		);
		await expect(
			canvas.queryByRole("heading", { name: /Windows workspace/ }),
		).not.toBeInTheDocument();
		await expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
	},
};
