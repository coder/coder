import type { Meta, StoryObj } from "@storybook/react";
import {
	MockBuildInfo,
	MockProvisioner,
	MockProvisioner2,
	MockProvisionerBuiltinKey,
	MockProvisionerKey,
	MockProvisionerPskKey,
	MockProvisionerWithTags,
	MockUserProvisioner,
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
				key: { ...MockProvisionerKey, id: "ジャイデン", name: "ジャイデン" },
				daemons: [MockProvisioner, MockProvisioner2],
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
				key: { ...MockProvisionerKey, id: "ケイラ", name: "ケイラ" },
				daemons: [
					{
						...MockProvisioner,
						tags: {
							...MockProvisioner.tags,
							都市: "ユタ",
							きっぷ: "yes",
							ちいさい: "no",
						},
					},
				],
			},
		],
	},
};

export const Empty: Story = {
	args: {
		provisioners: [],
	},
};
