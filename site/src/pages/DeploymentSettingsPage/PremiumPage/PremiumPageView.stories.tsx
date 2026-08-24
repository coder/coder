import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { PremiumPageView } from "./PremiumPageView";

const meta: Meta<typeof PremiumPageView> = {
	title: "pages/DeploymentSettingsPage/PremiumPageView",
	component: PremiumPageView,
	args: {
		hasLicense: false,
		isTrial: false,
		canRequestTrial: true,
		trialDaysRemaining: undefined,
		isSubmitting: false,
		onSubmit: fn(),
	},
};

export default meta;

type Story = StoryObj<typeof PremiumPageView>;

export const NoLicense: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The hero tracks the panel, so the trial pitch only appears here.
		await expect(
			canvas.getByRole("heading", {
				name: "Start a 30-day trial of Coder Premium",
				level: 2,
			}),
		).toBeInTheDocument();
		await expect(canvas.getByLabelText(/^Business email/)).toBeVisible();
		// The acknowledgement gates submission.
		await expect(
			canvas.getByRole("button", { name: "Start a trial" }),
		).toBeDisabled();
		// Requesting a trial replaces the old contact-sales upsell.
		await expect(
			canvas.queryByRole("link", { name: /contact sales/i }),
		).not.toBeInTheDocument();
	},
};

export const NoLicenseWithoutPermission: Story = {
	args: {
		canRequestTrial: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText("Contact your deployment administrator for Premium."),
		).toBeVisible();
		await expect(
			canvas.queryByLabelText(/^Business email/),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("button", { name: "Start a trial" }),
		).not.toBeInTheDocument();
	},
};

export const TrialActive: Story = {
	args: {
		hasLicense: true,
		isTrial: true,
		trialDaysRemaining: 23,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("heading", {
				name: "23 days remaining",
				level: 1,
			}),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: /contact sales/i }),
		).toHaveAttribute("href", "https://coder.com/contact/sales");
		await expect(
			canvas.queryByLabelText(/^Business email/),
		).not.toBeInTheDocument();
		// The upsell drops the secondary CTA and the feature checklist so it no
		// longer mirrors the trial request form.
		await expect(
			canvas.queryByRole("link", { name: "View licenses" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByText("24x7 global support with SLA"),
		).not.toBeInTheDocument();
	},
};

export const TrialActiveSingleDay: Story = {
	args: {
		hasLicense: true,
		isTrial: true,
		trialDaysRemaining: 1,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByText("1 day remaining")).toBeVisible();
	},
};

export const TrialExpiryUnavailable: Story = {
	args: {
		hasLicense: true,
		isTrial: true,
		trialDaysRemaining: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The count is the heading, so an unreadable expiry leaves no heading.
		await expect(canvas.queryByRole("heading")).not.toBeInTheDocument();
		await expect(canvas.queryByText(/remaining/)).not.toBeInTheDocument();
		await expect(canvas.queryByText(/NaN|undefined/)).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: /contact sales/i }),
		).toBeVisible();
	},
};

export const TrialExpired: Story = {
	args: {
		hasLicense: true,
		isTrial: true,
		trialDaysRemaining: -3,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// An elapsed trial must never render a negative count.
		await expect(canvas.queryByText(/remaining/)).not.toBeInTheDocument();
		await expect(canvas.queryByText(/-3/)).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: /contact sales/i }),
		).toBeVisible();
	},
};

export const LicenseActive: Story = {
	args: {
		hasLicense: true,
		isTrial: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Copy stays license-neutral: this state also covers Enterprise licenses.
		await expect(
			canvas.getByRole("heading", {
				name: "A license is already installed",
				level: 1,
			}),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "View licenses" }),
		).toHaveAttribute("href", "/deployment/licenses");
		// An installed license collapses the two-column pitch into the banner.
		await expect(
			canvas.queryByRole("heading", { level: 2 }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByLabelText(/^Business email/),
		).not.toBeInTheDocument();
	},
};
