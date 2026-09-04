import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type { TemplateBuilderBasesResponse } from "#/api/typesGenerated";
import { BaseTemplateParametersStep } from "./BaseTemplateParametersStep";

const baseId = "aws-ec2";

const basesData: TemplateBuilderBasesResponse = {
	bases: [
		{
			id: baseId,
			name: "AWS EC2",
			description: "Provision workspaces as AWS EC2 instances.",
			icon: "/icon/aws.svg",
			os: "linux",
			prerequisites: "",
			agents: [],
			variables: [
				{
					name: "region",
					type: "string",
					description: "AWS region to deploy into.",
					required: true,
					sensitive: false,
				},
				{
					name: "instance_type",
					type: "string",
					description: "EC2 instance type.",
					required: false,
					sensitive: false,
				},
			],
		},
	],
};

const meta: Meta<typeof BaseTemplateParametersStep> = {
	title: "pages/TemplateBuilder/BaseTemplateParametersStep",
	component: BaseTemplateParametersStep,
	args: {
		baseId,
		values: {},
		onChangeValues: fn(),
	},
	parameters: {
		queries: [
			{
				key: ["templateBuilder", "bases"],
				data: basesData,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof BaseTemplateParametersStep>;

export const Default: Story = {};

// With showErrors on and the required "region" field empty, the field is
// flagged invalid (red outline) while the optional field is not.
export const RequiredFieldError: Story = {
	args: {
		showErrors: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const region = await canvas.findByRole("textbox", { name: /region/ });
		await expect(region).toHaveAttribute("aria-invalid", "true");
		const instanceType = canvas.getByRole("textbox", {
			name: /instance_type/,
		});
		await expect(instanceType).toHaveAttribute("aria-invalid", "false");
	},
};
