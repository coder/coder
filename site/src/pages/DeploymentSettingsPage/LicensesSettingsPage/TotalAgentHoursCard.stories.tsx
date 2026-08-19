import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { MockAgentRuntimeHoursFeature } from "#/testHelpers/entities";
import { TotalAgentHoursCard } from "./TotalAgentHoursCard";

const meta: Meta<typeof TotalAgentHoursCard> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/TotalAgentHoursCard",
	component: TotalAgentHoursCard,
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			// 400 hours and 18 minutes: renders as 400.3.
			actual_ms: 400 * 3_600_000 + 18 * 60_000,
		},
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

// Radix mounts the role="tooltip" node only while the popover is open.
// The node is the visually hidden copy of the content, so there is no
// visibility to assert.
const expectTooltipText = async (
	body: ReturnType<typeof within>,
	text: RegExp,
) => {
	const tooltip = await body.findByRole("tooltip");
	expect(tooltip).toHaveTextContent(text);
};

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("400.3")).toBeInTheDocument();
		await expect(canvas.getByText(/Warning:/)).toBeInTheDocument();
		await expect(canvas.getByText("850")).toBeInTheDocument();
		await expect(canvas.getByText(/Allocation:/)).toBeInTheDocument();
		await expect(canvas.getByText("1,000")).toBeInTheDocument();
		await expect(canvas.queryByText(/Limit:/)).not.toBeInTheDocument();
		await expect(
			canvas.getByText("(June 1, 2026 – May 31, 2027)"),
		).toBeInTheDocument();
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
			...MockAgentRuntimeHoursFeature,
			soft_limit: undefined,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText(/Warning:/)).not.toBeInTheDocument();
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
			...MockAgentRuntimeHoursFeature,
			actual: 850,
			actual_ms: 850 * 3_600_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("850.0")).toBeInTheDocument();
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
			...MockAgentRuntimeHoursFeature,
			actual: 1000,
			actual_ms: 1000 * 3_600_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,000.0")).toBeInTheDocument();
		await expect(canvas.getByText("1,000")).toBeInTheDocument();
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
			...MockAgentRuntimeHoursFeature,
			actual: 1200,
			actual_ms: 1200 * 3_600_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,200.0")).toBeInTheDocument();
		await expect(canvas.getByText("1,000")).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 120% of your Total Agent hours for this license\. Contact sales to receive more Agent hours\./,
		);
	},
};

// The extra 6 minutes alone push the tenths value past the whole-hour
// allocation and flip the reached state.
export const ReachedAllocationByFraction: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			actual: 1000,
			actual_ms: 1000 * 3_600_000 + 6 * 60_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,000.1")).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 100% of your Total Agent hours for this license\. Contact sales to receive more Agent hours\./,
		);
	},
};

// 99.9% must never read as a false 100%.
export const NearAllocation: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			soft_limit: 999,
			actual: 999,
			actual_ms: 999 * 3_600_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("999.0")).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 99\.9% or more of your Total Agent hours for this license\. Agent sessions are still working normally, but you'll want to plan for the 100% limit\./,
		);
	},
};

export const MissingActual: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			actual: undefined,
			actual_ms: undefined,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("N/A")).toBeInTheDocument();
	},
};

// Missing usage data must not count as reaching a zero soft limit, so
// the tooltip stays informational.
export const MissingActualZeroSoftLimit: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			soft_limit: 0,
			actual: undefined,
			actual_ms: undefined,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("N/A")).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/^Total time agents have been working across all workspaces this license\. A soft-limit warning appears at 0%$/,
		);
	},
};

export const MissingUsagePeriod: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			usage_period: undefined,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("400.0")).toBeInTheDocument();
		await expect(
			canvas.queryByText("(June 1, 2026 – May 31, 2027)"),
		).not.toBeInTheDocument();
	},
};

export const HardCap: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			hard_limit: 1500,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("400.0")).toBeInTheDocument();
		await expect(canvas.getByText(/Warning:/)).toBeInTheDocument();
		await expect(canvas.getByText("850")).toBeInTheDocument();
		await expect(canvas.getByText(/Allocation:/)).toBeInTheDocument();
		await expect(canvas.getByText("1,000")).toBeInTheDocument();
		await expect(canvas.getByText(/Limit:/)).toBeInTheDocument();
		await expect(canvas.getByText("1,500")).toBeInTheDocument();
		await expect(
			canvas.queryByText(
				"Agent hours exceeded. Concurrent chats are now limited to 5.",
			),
		).not.toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/^Total time agents have been working across all workspaces this license\. A soft-limit warning appears at 85%$/,
		);
	},
};

export const HardCapBetweenSoftLimitAndLimit: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			hard_limit: 1500,
			actual: 900,
			actual_ms: 900 * 3_600_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("900.0")).toBeInTheDocument();
		await expect(
			canvas.queryByText(
				"Agent hours exceeded. Concurrent chats are now limited to 5.",
			),
		).not.toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 85% or more of your Total Agent hours for this license\. Agent sessions are still working normally, but you'll want to plan for the 100% limit\./,
		);
	},
};

export const HardCapBetweenLimitAndHardCap: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			hard_limit: 1500,
			actual: 1200,
			actual_ms: 1200 * 3_600_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,200.0")).toBeInTheDocument();
		await expect(
			canvas.queryByText(
				"Agent hours exceeded. Concurrent chats are now limited to 5.",
			),
		).not.toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 120% of your Total Agent hours for this license\. Contact sales to receive more Agent hours\./,
		);
	},
};

export const ReachedHardCap: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			hard_limit: 1500,
			actual: 1600,
			actual_ms: 1600 * 3_600_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,600.0")).toBeInTheDocument();
		await expect(
			canvas.getByText(
				"Agent hours exceeded. Concurrent chats are now limited to 5.",
			),
		).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 160% of your Total Agent hours for this license and reached the hard cap of 1,500 hours\. Contact sales to receive more Agent hours\./,
		);
	},
};

// The backend accepts a hard cap equal to the allocation.
export const ReachedCoincidentHardCap: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			hard_limit: 1000,
			actual: 1000,
			actual_ms: 1000 * 3_600_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,000.0")).toBeInTheDocument();
		await expect(
			canvas.getByText(
				"Agent hours exceeded. Concurrent chats are now limited to 5.",
			),
		).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 100% of your Total Agent hours for this license and reached the hard cap of 1,000 hours\. Contact sales to receive more Agent hours\./,
		);
	},
};

export const Disabled: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			enabled: false,
			entitlement: "not_entitled",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("heading", { name: "Total agent hours" }),
		).not.toBeInTheDocument();
	},
};

export const Unlimited: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			limit: undefined,
			soft_limit: undefined,
			actual: 1200,
			// 1,200 hours and 30 minutes: renders as 1,200.5.
			actual_ms: 1200 * 3_600_000 + 30 * 60_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,200.5")).toBeInTheDocument();
		await expect(canvas.queryByText(/Warning:/)).not.toBeInTheDocument();
		await expect(canvas.getByText(/Allocation:/)).toBeInTheDocument();
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
			...MockAgentRuntimeHoursFeature,
			// A negative limit can only come from a decoding bug: the
			// claim-level unlimited sentinel decodes to an omitted limit,
			// never a negative one.
			limit: -100,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Invalid license usage limits"),
		).toBeInTheDocument();
	},
};
