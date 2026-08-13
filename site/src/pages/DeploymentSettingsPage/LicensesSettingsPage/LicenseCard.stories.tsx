import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { expect, fn, waitFor, within } from "storybook/test";
import { MockLicenseResponse } from "#/testHelpers/entities";

import { LicenseCard } from "./LicenseCard";

const EXPIRED_DATE = dayjs("2000-01-01T12:00:00Z").unix();
const FUTURE_START_DATE = dayjs("2099-01-01T12:00:00Z").unix();

const meta: Meta<typeof LicenseCard> = {
	title: "pages/DeploymentSettingsPage/LicensesSettingsPage/LicenseCard",
	component: LicenseCard,
	args: {
		license: MockLicenseResponse[0],
		userLimitActual: 4,
		userLimitLimit: 10,
		onRemove: fn(),
		isRemoving: false,
	},
};

export default meta;
type Story = StoryObj<typeof LicenseCard>;

const getMetricValue = (canvas: ReturnType<typeof within>, label: string) =>
	canvas.getByText(label).parentElement?.nextElementSibling;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("#1")).toBeInTheDocument();
		// The Users header field and the Coder Workspaces product card show
		// the same seat usage.
		await expect(canvas.getAllByText("4 / 10")).toHaveLength(2);
		await expect(canvas.getByText("Enterprise")).toBeInTheDocument();
		await expect(canvas.getByText("Standard")).toBeInTheDocument();
		await expect(canvas.getByText("Products")).toBeInTheDocument();
		await expect(canvas.getByText("Coder Workspaces")).toBeInTheDocument();
		// Enterprise licenses do not get the Coder Agents product.
		await expect(canvas.queryByText("Coder Agents")).not.toBeInTheDocument();
	},
};

export const CollapsesProducts: Story = {
	play: async ({ canvasElement, userEvent }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Products")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: /#1/ }));
		await waitFor(() =>
			expect(canvas.queryByText("Products")).not.toBeInTheDocument(),
		);
		await userEvent.click(canvas.getByRole("button", { name: /#1/ }));
		await waitFor(() => expect(canvas.getByText("Products")).toBeVisible());
	},
};

export const Trial: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				trial: true,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Premium")).toBeInTheDocument();
		const typeLabel = canvas.getByText("Type");
		await expect(typeLabel.nextElementSibling).toHaveTextContent("Trial");
	},
};

export const UnlimitedUsers: Story = {
	args: {
		userLimitLimit: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getAllByText("4 / Unlimited")).toHaveLength(2);
	},
};

export const UsesLicenseUserLimit: Story = {
	args: {
		license: {
			...MockLicenseResponse[0],
			claims: {
				...MockLicenseResponse[0].claims,
				features: {
					...MockLicenseResponse[0].claims.features,
					user_limit: 3,
				},
			},
		},
		userLimitActual: 1,
		userLimitLimit: 100,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getAllByText("1 / 3")).toHaveLength(2);
	},
};

export const Premium: Story = {
	args: {
		license: MockLicenseResponse[1],
		// The backend grandfathers premium licenses without agent hour
		// claims into a zero-hour allocation, so the merged entitlement is
		// always present: disabled, zero limit, usage measured.
		agentRuntimeHoursFeature: {
			enabled: false,
			entitlement: "entitled",
			limit: 0,
			actual: 137,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// A Premium license with no agent hours allocation shows the Coder
		// Agents upgrade card, including deployment-wide usage.
		await expect(canvas.getByText("Coder Agents")).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Max concurrent chats"),
		).toHaveTextContent("5");
		await expect(canvas.getByText(/Agent hours used/)).toHaveTextContent(
			"Agent hours used: 137",
		);
		const upgrade = canvas.getByRole("link", { name: "Upgrade" });
		await expect(upgrade).toHaveAttribute("href", "mailto:sales@coder.com");
	},
};

const premiumLicenseWithAgentHours = (allocation: number) => ({
	...MockLicenseResponse[1],
	claims: {
		...MockLicenseResponse[1].claims,
		features: {
			...MockLicenseResponse[1].claims.features,
			agent_runtime_hours_allocation: allocation,
			...(allocation > 0
				? {
						agent_runtime_hours_limit_soft: Math.floor(allocation * 0.8),
						agent_runtime_hours_limit_hard: Math.floor(allocation * 1.25),
					}
				: {}),
		},
	},
});

export const PremiumWithAgentHours: Story = {
	args: {
		license: premiumLicenseWithAgentHours(20000),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			actual: 16264,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Active")).toBeInTheDocument();
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"16,264 / 20,000",
		);
		await expect(getMetricValue(canvas, "Concurrent chats")).toHaveTextContent(
			"Unlimited",
		);
		await expect(
			canvas.getByRole("link", { name: "Manage usage" }),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: "Agent settings" }),
		).toBeInTheDocument();
	},
};

export const PremiumWithAgentHoursExceeded: Story = {
	args: {
		license: premiumLicenseWithAgentHours(20000),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			actual: 21000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Agent hours exceeded")).toBeInTheDocument();
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"21,000 / 20,000",
		);
		// Concurrency is only capped once the hard limit is reached.
		await expect(getMetricValue(canvas, "Concurrent chats")).toHaveTextContent(
			"Unlimited",
		);
	},
};

export const PremiumWithAgentHoursHardLimitExceeded: Story = {
	args: {
		license: premiumLicenseWithAgentHours(20000),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			actual: 25000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Hard limit exceeded")).toBeInTheDocument();
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"25,000 / 20,000",
		);
		await expect(getMetricValue(canvas, "Concurrent chats")).toHaveTextContent(
			"5",
		);
	},
};

export const PremiumWithUnlimitedAgentHours: Story = {
	args: {
		license: premiumLicenseWithAgentHours(-1),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 16264,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Active")).toBeInTheDocument();
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"Unlimited",
		);
		await expect(getMetricValue(canvas, "Concurrent chats")).toHaveTextContent(
			"Unlimited",
		);
	},
};

export const LowerAgentHoursCardUsesMergedEntitlement: Story = {
	args: {
		license: premiumLicenseWithAgentHours(10000),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			actual: 16264,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Usage belongs to the winning 20,000-hour license, so this card
		// shows no usage and no overage.
		await expect(getMetricValue(canvas, "Total Agent hours")).toHaveTextContent(
			"\u2014 / 10,000",
		);
		await expect(
			canvas.queryByText("Agent hours exceeded"),
		).not.toBeInTheDocument();
	},
};

export const PremiumWithAIGovernance: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 750,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/add-ons/i)).toBeInTheDocument();
		await expect(canvas.getByText(/ai governance/i)).toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("750 / 1,000");
	},
};

export const PremiumWithoutAIGovernanceAddOn: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 100,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("Add-ons")).not.toBeInTheDocument();
		await expect(canvas.queryByText("AI Governance")).not.toBeInTheDocument();
	},
};

export const Expired: Story = {
	args: {
		license: MockLicenseResponse[3],
	},
};

export const ExceededUserLimit: Story = {
	args: {
		userLimitActual: 15,
		userLimitLimit: 10,
	},
};

export const ExceededAIGovernance: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 1200,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Add-on exceeded")).toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("1,200 / 1,000");
	},
};

export const ExpiredAIGovernanceOverageShowsExpired: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				license_expires: EXPIRED_DATE,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 1200,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Expired")).toBeInTheDocument();
		await expect(canvas.queryByText("Add-on exceeded")).not.toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("—");
		await expect(seatsValue).toHaveTextContent("/ 1,000");
	},
};

export const ExpiredAIGovernanceInGracePeriodShowsExceeded: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				license_expires: EXPIRED_DATE,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "grace_period",
			actual: 1200,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Add-on exceeded")).toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("1,200 / 1,000");
	},
};

export const NotYetValid: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				nbf: FUTURE_START_DATE,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/Not started/)).toBeInTheDocument();
	},
};

export const FutureAIGovernanceOverageShowsStartsOn: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				nbf: FUTURE_START_DATE,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 1200,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/Not started/)).toBeInTheDocument();
		await expect(canvas.queryByText("Add-on exceeded")).not.toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("—");
		await expect(seatsValue).toHaveTextContent("/ 1,000");
	},
};

export const FutureAIGovernanceUsageShowsNoCurrentSeats: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				nbf: FUTURE_START_DATE,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("—");
		await expect(seatsValue).toHaveTextContent("/ 1,000");
		await expect(seatsValue).not.toHaveTextContent("0 / 1,000");
	},
};

export const LowerLimitCardUsesMergedEntitlement: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 500,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 750,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("—");
		await expect(seatsValue).toHaveTextContent("/ 500");
		await expect(seatsValue).not.toHaveTextContent("750 / 500");
		await expect(canvas.queryByText("Add-on exceeded")).not.toBeInTheDocument();
	},
};

export const EnterpriseDoesNotShowAIGovernanceAddOn: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				feature_set: "enterprise",
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 750,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("Add-ons")).not.toBeInTheDocument();
		await expect(canvas.queryByText("AI add-on")).not.toBeInTheDocument();
	},
};
