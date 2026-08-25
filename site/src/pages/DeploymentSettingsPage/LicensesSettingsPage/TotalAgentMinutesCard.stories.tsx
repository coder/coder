import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { MockAgentRuntimeHoursFeature } from "#/testHelpers/entities";
import { TotalAgentMinutesCard } from "./TotalAgentMinutesCard";

const meta: Meta<typeof TotalAgentMinutesCard> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/TotalAgentMinutesCard",
	component: TotalAgentMinutesCard,
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			// 400 hours and 18 minutes: renders as 24,018 minutes.
			actual_ms: 400 * 3_600_000 + 18 * 60_000,
		},
	},
};

export default meta;
type Story = StoryObj<typeof TotalAgentMinutesCard>;

const hoverInfoIcon = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.hover(
		canvas.getByRole("button", { name: "Total agent minutes information" }),
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
		await expect(canvas.getByText("24,018")).toBeInTheDocument();
		await expect(canvas.getByText(/Warning:/)).toBeInTheDocument();
		await expect(canvas.getByText("51,000")).toBeInTheDocument();
		await expect(canvas.getByText(/Allocation:/)).toBeInTheDocument();
		await expect(canvas.getByText("60,000")).toBeInTheDocument();
		await expect(canvas.queryByText(/Limit:/)).not.toBeInTheDocument();
		await expect(
			canvas.getByText("(June 1, 2026 - May 31, 2027)"),
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
		await expect(canvas.getAllByText("51,000")).toHaveLength(2);
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 85% or more of your Total Agent minutes for this license\. Agent sessions are still working normally, but you'll want to plan for the 100% limit\./,
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
		await expect(canvas.getAllByText("60,000")).toHaveLength(2);
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 100% of your Total Agent minutes for this license\. Contact sales to receive more Agent minutes\./,
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
		await expect(canvas.getByText("72,000")).toBeInTheDocument();
		await expect(canvas.getByText("60,000")).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 120% of your Total Agent minutes for this license\. Contact sales to receive more Agent minutes\./,
		);
	},
};

// The extra 6 minutes alone push usage past the allocation and flip the
// reached state.
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
		await expect(canvas.getByText("60,006")).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 100% of your Total Agent minutes for this license\. Contact sales to receive more Agent minutes\./,
		);
	},
};

// Integer claims such as 29/100 must not underflow to 28.9% after
// flooring a binary ratio ((29 / 100) * 100 === 28.999...).
export const SoftLimitExactPercent: Story = {
	args: {
		feature: {
			...MockAgentRuntimeHoursFeature,
			limit: 100,
			soft_limit: 29,
			actual: 10,
			actual_ms: 10 * 3_600_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("600")).toBeInTheDocument();
		await expect(canvas.getByText("1,740")).toBeInTheDocument();
		await expect(canvas.getByText("6,000")).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/Total time agents have been working across all workspaces this license\. A soft-limit warning appears at 29%/,
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
		await expect(canvas.getAllByText("59,940")).toHaveLength(2);
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 99\.9% or more of your Total Agent minutes for this license\. Agent sessions are still working normally, but you'll want to plan for the 100% limit\./,
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
		await expect(canvas.getByText("24,000")).toBeInTheDocument();
		await expect(
			canvas.queryByText("(June 1, 2026 - May 31, 2027)"),
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
		await expect(canvas.getByText("24,000")).toBeInTheDocument();
		await expect(canvas.getByText(/Warning:/)).toBeInTheDocument();
		await expect(canvas.getByText("51,000")).toBeInTheDocument();
		await expect(canvas.getByText(/Allocation:/)).toBeInTheDocument();
		await expect(canvas.getByText("60,000")).toBeInTheDocument();
		await expect(canvas.getByText(/Limit:/)).toBeInTheDocument();
		await expect(canvas.getByText("90,000")).toBeInTheDocument();
		await expect(
			canvas.queryByText(
				"Agent minutes limit reached. Concurrent chats are now limited to 5.",
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
		await expect(canvas.getByText("54,000")).toBeInTheDocument();
		await expect(
			canvas.queryByText(
				"Agent minutes limit reached. Concurrent chats are now limited to 5.",
			),
		).not.toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 85% or more of your Total Agent minutes for this license\. Agent sessions are still working normally, but you'll want to plan for the 100% limit\./,
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
		await expect(canvas.getByText("72,000")).toBeInTheDocument();
		await expect(
			canvas.queryByText(
				"Agent minutes limit reached. Concurrent chats are now limited to 5.",
			),
		).not.toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 120% of your Total Agent minutes for this license\. Contact sales to receive more Agent minutes\./,
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
		await expect(canvas.getByText("96,000")).toBeInTheDocument();
		await expect(
			canvas.getByText(
				"Agent minutes limit reached. Concurrent chats are now limited to 5.",
			),
		).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 160% of your Total Agent minutes for this license and reached the hard cap of 90,000 minutes\. Contact sales to receive more Agent minutes\./,
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
		await expect(canvas.getAllByText("60,000")).toHaveLength(3);
		await expect(
			canvas.getByText(
				"Agent minutes limit reached. Concurrent chats are now limited to 5.",
			),
		).toBeInTheDocument();
		const body = await hoverInfoIcon(canvasElement);
		await expectTooltipText(
			body,
			/You've used 100% of your Total Agent minutes for this license and reached the hard cap of 60,000 minutes\. Contact sales to receive more Agent minutes\./,
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
			canvas.queryByRole("heading", { name: "Total agent minutes" }),
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
			// 1,200 hours and 30 minutes: renders as 72,030 minutes.
			actual_ms: 1200 * 3_600_000 + 30 * 60_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("72,030")).toBeInTheDocument();
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
