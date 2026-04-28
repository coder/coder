import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { SecretRequirementStatus } from "#/api/typesGenerated";
import { SecretsTable } from "./SecretsTable";

const meta: Meta<typeof SecretsTable> = {
	title: "modules/workspaces/SecretsTable",
	component: SecretsTable,
};

export default meta;
type Story = StoryObj<typeof SecretsTable>;

const awsAccessKey: SecretRequirementStatus = {
	env: "AWS_ACCESS_KEY_ID",
	help_message: "Access key used to provision AWS resources.",
	satisfied: true,
};

const awsSecretKey: SecretRequirementStatus = {
	env: "AWS_SECRET_ACCESS_KEY",
	help_message:
		"Secret key paired with AWS_ACCESS_KEY_ID. This description is intentionally long so the row truncates the visible text and relies on the information tooltip for the complete value.",
	satisfied: false,
};

const kubeConfig: SecretRequirementStatus = {
	file: "~/.kube/config",
	help_message: "Kubeconfig file used to connect to the target cluster.",
	satisfied: false,
};

const gcpCredentials: SecretRequirementStatus = {
	file: "~/.config/gcloud/application_default_credentials.json",
	help_message: "Application default credentials for Google Cloud.",
	satisfied: true,
};

const expectSecretsTable = async (
	canvasElement: HTMLElement,
	requirements: readonly SecretRequirementStatus[],
) => {
	const canvas = within(canvasElement);
	const table = canvas.getByRole("table", { name: /required secrets/i });
	const rows = canvas.getAllByRole("row");
	const missingCount = requirements.filter(
		(requirement) => !requirement.satisfied,
	).length;

	expect(table).toBeVisible();
	expect(rows).toHaveLength(requirements.length);

	const expectedRequirements = [
		...requirements.filter((requirement) => !requirement.satisfied),
		...requirements.filter((requirement) => requirement.satisfied),
	];
	for (const [index, requirement] of expectedRequirements.entries()) {
		expect(rows[index]).toHaveTextContent(secretRequirementTarget(requirement));
	}

	const requirementsWithHelp = requirements.filter(
		(requirement) => requirement.help_message,
	);
	const descriptionButtons = canvas.getAllByRole("button", {
		name: /show full description/i,
	});
	expect(descriptionButtons).toHaveLength(requirementsWithHelp.length);

	await userEvent.hover(descriptionButtons[0]);
	const tooltip = await within(document.body).findByRole("tooltip");
	expect(tooltip).toHaveTextContent(expectedRequirements[0].help_message);
	await userEvent.unhover(descriptionButtons[0]);

	const manageSecretsLink = canvas.getByRole("link", {
		name: /manage secrets/i,
	});
	expect(manageSecretsLink).toBeVisible();
	expect(manageSecretsLink).toHaveAttribute(
		"href",
		expect.stringContaining("/reference/cli/secret_create"),
	);
	expect(manageSecretsLink).toHaveAttribute("target", "_blank");

	if (missingCount >= 1) {
		const addSecretLinks = canvas.getAllByRole("link", {
			name: /add secret/i,
		});
		expect(addSecretLinks).toHaveLength(missingCount);
		for (const link of addSecretLinks) {
			expect(link).toHaveAttribute(
				"href",
				expect.stringContaining("/reference/cli/secret_create"),
			);
			expect(link).toHaveAttribute("target", "_blank");
		}
		return;
	}

	expect(canvas.queryByRole("link", { name: /add secret/i })).toBeNull();
};

const secretRequirementTarget = (requirement: SecretRequirementStatus) => {
	return requirement.file || requirement.env || "Unknown secret";
};

export const AllSatisfied: Story = {
	args: {
		requirements: [awsAccessKey, gcpCredentials],
	},
	play: async ({ canvasElement, args }) => {
		await expectSecretsTable(canvasElement, args.requirements);
	},
};

export const Mixed: Story = {
	args: {
		requirements: [awsAccessKey, awsSecretKey, kubeConfig, gcpCredentials],
	},
	play: async ({ canvasElement, args }) => {
		await expectSecretsTable(canvasElement, args.requirements);
	},
};

export const AllMissing: Story = {
	args: {
		requirements: [
			{ ...awsAccessKey, satisfied: false },
			awsSecretKey,
			kubeConfig,
		],
	},
	play: async ({ canvasElement, args }) => {
		await expectSecretsTable(canvasElement, args.requirements);
	},
};

export const SingleRow: Story = {
	args: {
		requirements: [awsSecretKey],
	},
	play: async ({ canvasElement, args }) => {
		await expectSecretsTable(canvasElement, args.requirements);
	},
};
