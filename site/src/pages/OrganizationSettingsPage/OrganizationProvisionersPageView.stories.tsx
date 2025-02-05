import type { Meta, StoryObj } from "@storybook/react";
import { screen, userEvent } from "@storybook/test";
import {
	MockBuildInfo,
	MockProvisioner,
	MockProvisioner2,
	MockProvisionerBuiltinKey,
	MockProvisionerKey,
	MockProvisionerPskKey,
	MockProvisionerUserAuthKey,
	MockProvisionerWithTags,
	MockUserProvisioner,
	mockApiError,
} from "testHelpers/entities";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

const meta: Meta<typeof OrganizationProvisionersPageView> = {
	title: "pages/OrganizationProvisionersPage",
	component: OrganizationProvisionersPageView,
	args: {
		buildInfo: MockBuildInfo,
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationProvisionersPageView>;

export const Provisioners: Story = {
	args: {
		provisioners: [
			{
				key: MockProvisionerBuiltinKey,
				daemons: [MockProvisioner, MockProvisioner2],
			},
			{
				key: MockProvisionerPskKey,
				daemons: [
					MockProvisioner,
					MockUserProvisioner,
					MockProvisionerWithTags,
				],
			},
			{
				key: MockProvisionerPskKey,
				daemons: [MockProvisioner, MockProvisioner2],
			},
			{
				key: { ...MockProvisionerKey, id: "ジェイデン", name: "ジェイデン" },
				daemons: [
					MockProvisioner,
					{ ...MockProvisioner2, tags: { scope: "organization", owner: "" } },
				],
			},
			{
				key: { ...MockProvisionerKey, id: "ベン", name: "ベン" },
				daemons: [
					MockProvisioner,
					{
						...MockProvisioner2,
						version: "2.0.0",
						api_version: "1.0",
					},
				],
			},
			{
				key: {
					...MockProvisionerKey,
					id: "ケイラ",
					name: "ケイラ",
					tags: {
						...MockProvisioner.tags,
						都市: "ユタ",
						きっぷ: "yes",
						ちいさい: "no",
					},
				},
				daemons: Array.from({ length: 117 }, (_, i) => ({
					...MockProvisioner,
					id: `ケイラ-${i}`,
					name: `ケイラ-${i}`,
				})),
			},
			{
				key: MockProvisionerUserAuthKey,
				daemons: [
					MockUserProvisioner,
					{
						...MockUserProvisioner,
						id: "mock-user-provisioner-2",
						name: "Test User Provisioner 2",
					},
				],
			},
		],
	},
	play: async ({ step }) => {
		await step("open all details", async () => {
			const expandButtons = await screen.findAllByRole("button", {
				name: "Show provisioner details",
			});
			for (const it of expandButtons) {
				await userEvent.click(it);
			}
		});

		await step("close uninteresting/large details", async () => {
			const collapseButtons = await screen.findAllByRole("button", {
				name: "Hide provisioner details",
			});

			await userEvent.click(collapseButtons[2]);
			await userEvent.click(collapseButtons[3]);
			await userEvent.click(collapseButtons[5]);
		});

		await step("show version popover", async () => {
			const outOfDate = await screen.findByText("Out of date");
			await userEvent.hover(outOfDate);
		});
	},
};

export const Empty: Story = {
	args: {
		provisioners: [],
	},
};

export const WithError: Story = {
	args: {
		error: mockApiError({
			message: "Fern is mad",
			detail: "Frieren slept in and didn't get groceries",
		}),
	},
};

export const Paywall: Story = {
	args: {
		showPaywall: true,
	},
};
