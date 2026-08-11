import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { Feature } from "#/api/typesGenerated";
import { TotalAgentHoursCard } from "./TotalAgentHoursCard";

const meta: Meta<typeof TotalAgentHoursCard> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/TotalAgentHoursCard",
	component: TotalAgentHoursCard,
	args: {
		feature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
			soft_limit: 850,
			actual: 400,
		} satisfies Feature,
	},
};

export default meta;
type Story = StoryObj<typeof TotalAgentHoursCard>;

const hoverInfoIcon = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.hover(
		canvas.getByRole("button", { name: "Total agent hours information" }),
	);
	return within(canvasElement.ownerDocument.body);
};

// Tooltip text appears in both the tooltip popover and its hidden
// accessibility duplicate, so assertions match all occurrences.
const expectTooltipText = async (
	body: ReturnType<typeof within>,
	text: RegExp,
) => {
	const matches = await body.findAllByText(text);
	expect(matches.length).toBeGreaterThanOrEqual(1);
};

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("400")).toBeInTheDocument();
		await expect(canvas.getByText("1,000")).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/Total time agents have been working across all workspaces this license\. A soft-limit warning appears at 85%/,
		);
	},
};

export const NoSoftLimit: Story = {
	args: {
		feature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
			actual: 400,
		} satisfies Feature,
	},
	play: async ({ canvasElement }) => {
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/^Total time agents have been working across all workspaces this license\.$/,
		);
	},
};

export const ReachedSoftLimit: Story = {
	args: {
		feature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
			soft_limit: 850,
			actual: 850,
		} satisfies Feature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("850")).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 85% or more of your Total Agent hours for this license\. Agent sessions are still working normally, but you'll want to plan for the 100% limit\./,
		);
	},
};

export const ReachedAllocation: Story = {
	args: {
		feature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
			soft_limit: 850,
			actual: 1000,
		} satisfies Feature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getAllByText("1,000")).toHaveLength(2);
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 100% of your Total Agent hours for this license\. Contact sales to receive more Agent hours\./,
		);
	},
};

export const OverAllocation: Story = {
	args: {
		feature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
			soft_limit: 850,
			actual: 1200,
		} satisfies Feature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,200")).toBeInTheDocument();
		await expect(canvas.getByText("1,000")).toBeInTheDocument();
	},
};

export const MissingActual: Story = {
	args: {
		feature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
			soft_limit: 850,
		} satisfies Feature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("\u2014")).toBeInTheDocument();
	},
};

export const Disabled: Story = {
	args: {
		feature: {
			enabled: false,
			entitlement: "not_entitled",
		} satisfies Feature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("heading", { name: "Total agent hours" }),
		).not.toBeInTheDocument();
	},
};

// An enabled feature with the limit omitted means the license grants
// unlimited runtime hours: the bar renders full and neutral with no
// threshold copy in the tooltip, mirroring SeatUsageBarCard's unlimited
// state.
export const Unlimited: Story = {
	args: {
		feature: {
			enabled: true,
			entitlement: "entitled",
			actual: 1200,
		} satisfies Feature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,200")).toBeInTheDocument();
		await expect(canvas.getByText("Unlimited")).toBeInTheDocument();
		await expect(
			canvas.queryByText("Invalid license usage limits"),
		).not.toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/^Total time agents have been working across all workspaces this license\.$/,
		);
	},
};

export const ErrorInvalidLimit: Story = {
	args: {
		feature: {
			enabled: true,
			entitlement: "entitled",
			// A negative limit can only come from a decoding bug: the
			// claim-level unlimited sentinel decodes to an omitted limit,
			// never a negative one.
			limit: -100,
			actual: 100,
		} satisfies Feature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Invalid license usage limits"),
		).toBeInTheDocument();
	},
};
