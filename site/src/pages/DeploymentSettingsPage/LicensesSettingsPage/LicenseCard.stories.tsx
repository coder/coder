import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
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

const getHeaderValue = (canvas: ReturnType<typeof within>, label: string) =>
	canvas.getByText(label).nextElementSibling;

const getIncludedProducts = (
	canvas: ReturnType<typeof within>,
	label: string,
) =>
	canvas.queryByRole("group", {
		name: (accessibleName: string) => accessibleName === label,
	});

const productCardTitles = (canvasElement: HTMLElement) =>
	Array.from(
		canvasElement.querySelectorAll(
			".license-card [data-testid='license-product-card-title']",
		),
	).map((element) => element.textContent);

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("#1")).toBeInTheDocument();
		await expect(canvas.getAllByText("4 / 10")).toHaveLength(2);
		await expect(canvas.getByText("Enterprise")).toBeInTheDocument();
		await expect(canvas.getByText("Standard")).toBeInTheDocument();
		await expect(canvas.getByText("Products")).toBeInTheDocument();
		await expect(canvas.getByText("Coder Workspaces")).toBeInTheDocument();
		// Licenses without an allocation still show the Coder Agents
		// upsell card.
		await expect(canvas.getByText("Coder Agents")).toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: "Try unlimited for 30 days" }),
		).toBeInTheDocument();
		await expect(
			getIncludedProducts(canvas, "Workspaces"),
		).not.toBeInTheDocument();
		await expect(
			getIncludedProducts(canvas, "Workspaces + Agents"),
		).not.toBeInTheDocument();
	},
};

export const CollapsesProducts: Story = {
	play: async ({ canvasElement }) => {
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
		await expect(
			getIncludedProducts(canvas, "Workspaces + Agents"),
		).toBeInTheDocument();
		await expect(canvas.getByText("Coder Agents Trial")).toBeInTheDocument();
		await expect(canvas.getByRole("link", { name: "Upgrade" })).toHaveAttribute(
			"href",
			"mailto:sales@coder.com",
		);
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
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				// A seat-limit claim without addons is not enough to show
				// the AI Governance add-on; that requires addons: ["ai_governance"].
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
		// Premium licenses without agent hour claims are grandfathered
		// into a zero-hour allocation, so the merged entitlement exists.
		agentRuntimeHoursFeature: {
			enabled: false,
			entitlement: "entitled",
			limit: 0,
			actual: 137,
			// 137 hours and 18 minutes: renders as 8,238 minutes.
			actual_ms: 137 * 3_600_000 + 18 * 60_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Coder Agents")).toBeInTheDocument();
		await expect(getIncludedProducts(canvas, "Workspaces")).toBeInTheDocument();
		await expect(
			getIncludedProducts(canvas, "Workspaces + Agents"),
		).not.toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Max concurrent agents"),
		).toHaveTextContent("5");
		await expect(
			getMetricValue(canvas, "Agent minutes used"),
		).toHaveTextContent("8,238");
		await expect(
			canvas.getByRole("link", { name: "Try unlimited for 30 days" }),
		).toHaveAttribute("href", "/deployment/premium");
		await expect(canvas.queryByText("AI Governance")).not.toBeInTheDocument();
	},
};

// Issued-at of the license supplying the merged entitlement; only the
// license whose iat/nbf/exp reproduce the merged period shows usage.
const WINNING_ISSUED_AT = dayjs("2026-01-01T12:00:00Z");
const winningUsagePeriod = {
	issued_at: WINNING_ISSUED_AT.toISOString(),
	start: WINNING_ISSUED_AT.toISOString(),
	end: WINNING_ISSUED_AT.add(1, "year").toISOString(),
};

const premiumLicenseWithAgentHours = (
	allocation: number,
	issuedAt = WINNING_ISSUED_AT,
) => ({
	...MockLicenseResponse[1],
	claims: {
		...MockLicenseResponse[1].claims,
		iat: issuedAt.unix(),
		nbf: issuedAt.unix(),
		exp: issuedAt.add(1, "year").unix(),
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
			actual: 12264,
			// 12,264 hours and 18 minutes: renders as 735,858 minutes,
			// below the 960,000-minute advisory soft limit.
			actual_ms: 12_264 * 3_600_000 + 18 * 60_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Active")).toBeInTheDocument();
		// An allocation without a seat claim is an agents-only license.
		await expect(
			getIncludedProducts(canvas, "Coder Agents"),
		).toBeInTheDocument();
		await expect(getHeaderValue(canvas, "Minutes")).toHaveTextContent(
			"735,858 / 1,200,000",
		);
		await expect(canvas.queryByText("Users")).not.toBeInTheDocument();
		await expect(
			canvas.queryByText("Coder Workspaces"),
		).not.toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("735,858 / 1,200,000");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
		await expect(
			canvas.queryByRole("link", { name: "Manage usage" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: "View docs" }),
		).toBeInTheDocument();
	},
};

export const AgentHoursWithSeatsKeepsUsersColumn: Story = {
	args: {
		license: (() => {
			const license = premiumLicenseWithAgentHours(20000);
			return {
				...license,
				claims: {
					...license.claims,
					features: { ...license.claims.features, user_limit: 10 },
				},
			};
		})(),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			actual: 12264,
			actual_ms: 12_264 * 3_600_000 + 18 * 60_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Users")).toBeInTheDocument();
		await expect(canvas.queryByText("Minutes")).not.toBeInTheDocument();
		await expect(canvas.getByText("Coder Workspaces")).toBeInTheDocument();
		await expect(
			getIncludedProducts(canvas, "Workspaces + Agents"),
		).toBeInTheDocument();
	},
};

export const PremiumWithAgentHoursSoftLimitReached: Story = {
	args: {
		license: premiumLicenseWithAgentHours(20000),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			actual: 16264,
			// 16,264 hours and 18 minutes: renders as 975,858 minutes, at
			// or above the 960,000-minute advisory soft limit.
			actual_ms: 16_264 * 3_600_000 + 18 * 60_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Active")).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("975,858 / 1,200,000");
		await expect(canvas.getByRole("status")).toHaveTextContent(
			"Approaching minutes limit",
		);
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
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
			actual_ms: 21_000 * 3_600_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Agent minutes exceeded"),
		).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("1,260,000 / 1,200,000");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
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
			actual_ms: 25_000 * 3_600_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Limit exceeded")).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("1,500,000 / 1,200,000");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"5",
		);
		await expect(canvas.getByRole("status")).toHaveTextContent("Limit reached");
	},
};

export const PremiumWithAgentHoursAtAllocation: Story = {
	args: {
		license: premiumLicenseWithAgentHours(20000),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			// Usage equal to the allocation is already over: the backend
			// reports the allocation as reached at this exact boundary.
			actual: 20000,
			actual_ms: 20_000 * 3_600_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Agent minutes exceeded"),
		).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("1,200,000 / 1,200,000");
	},
};

export const PremiumWithAgentHoursExceededByFraction: Story = {
	args: {
		license: premiumLicenseWithAgentHours(20000),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			// The extra 6 minutes push usage past the allocation, so the
			// display shows the overage.
			actual: 20000,
			actual_ms: 20_000 * 3_600_000 + 6 * 60_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Agent minutes exceeded"),
		).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("1,200,006 / 1,200,000");
	},
};

export const PremiumWithUnlimitedAgentHours: Story = {
	args: {
		license: premiumLicenseWithAgentHours(-1),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 16264,
			actual_ms: 16_264 * 3_600_000 + 18 * 60_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Active")).toBeInTheDocument();
		await expect(
			getIncludedProducts(canvas, "Coder Agents"),
		).toBeInTheDocument();
		await expect(getHeaderValue(canvas, "Minutes")).toHaveTextContent(
			"975,858 / Unlimited",
		);
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("Unlimited");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
	},
};

export const LowerAgentHoursCardUsesMergedEntitlement: Story = {
	args: {
		license: premiumLicenseWithAgentHours(
			10000,
			WINNING_ISSUED_AT.subtract(1, "year"),
		),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			actual: 16264,
			actual_ms: 16_264 * 3_600_000 + 18 * 60_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("\u2014 / 600,000");
		await expect(
			canvas.queryByText("Agent minutes exceeded"),
		).not.toBeInTheDocument();
	},
};

export const ReplacedDuplicateAllocationShowsNoUsage: Story = {
	args: {
		// Same allocation as the winning renewal but an older usage
		// period, so the merged usage does not belong to this license.
		license: premiumLicenseWithAgentHours(
			20000,
			WINNING_ISSUED_AT.subtract(1, "year"),
		),
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			actual: 26000,
			actual_ms: 26_000 * 3_600_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Active")).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("\u2014 / 1,200,000");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
	},
};

const sameIssuedAtShorterTermLicense = (() => {
	const license = premiumLicenseWithAgentHours(20000);
	return {
		...license,
		claims: {
			...license.claims,
			exp: WINNING_ISSUED_AT.add(6, "month").unix(),
		},
	};
})();

export const SameIssuedAtDifferentTermEndShowsNoUsage: Story = {
	args: {
		// Same iat and allocation as the winning license but a shorter
		// term; the backend tie-breaks equal issued-at on the period end.
		license: sameIssuedAtShorterTermLicense,
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			actual: 26000,
			actual_ms: 26_000 * 3_600_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Active")).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("\u2014 / 1,200,000");
		await expect(getMetricValue(canvas, "Concurrent agents")).toHaveTextContent(
			"Unlimited",
		);
	},
};

const enterpriseLicenseWithAgentHours = (() => {
	const license = premiumLicenseWithAgentHours(20000);
	return {
		...license,
		claims: {
			...license.claims,
			feature_set: "enterprise",
		},
	};
})();

export const EnterpriseWithAgentHours: Story = {
	args: {
		// Runtime hour claims apply to any feature set, so an Enterprise
		// license with an allocation renders the Coder Agents product.
		license: enterpriseLicenseWithAgentHours,
		agentRuntimeHoursFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 20000,
			soft_limit: 16000,
			hard_limit: 25000,
			actual: 12264,
			actual_ms: 12_264 * 3_600_000 + 18 * 60_000,
			usage_period: winningUsagePeriod,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Enterprise")).toBeInTheDocument();
		await expect(
			getIncludedProducts(canvas, "Workspaces"),
		).not.toBeInTheDocument();
		await expect(
			getIncludedProducts(canvas, "Coder Agents"),
		).toBeInTheDocument();
		await expect(
			getMetricValue(canvas, "Total Agent minutes"),
		).toHaveTextContent("735,858 / 1,200,000");
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
		// The add-on card lives in the Products grid, so no separate
		// Add-ons section exists.
		await expect(canvas.queryByText("Add-ons")).not.toBeInTheDocument();
		// Matches both the included-products line and the add-on card title.
		await expect(canvas.getAllByText(/ai governance/i)).toHaveLength(2);
		await expect(
			getIncludedProducts(canvas, "Workspaces + AI Governance"),
		).toBeInTheDocument();
		await expect(productCardTitles(canvasElement)).toEqual([
			"Coder Workspaces",
			"AI Governance",
			"Coder Agents",
		]);
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("750 / 1,000");
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
		await expect(canvas.queryByText("AI Governance")).not.toBeInTheDocument();
		await expect(canvas.queryByText("Seats")).not.toBeInTheDocument();
	},
};
