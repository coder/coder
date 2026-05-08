import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { SecretRequirementStatus } from "#/api/typesGenerated";
import { SecretsTable, secretRequirementLabel } from "./SecretsTable";

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
	const rows = within(table).getAllByRole("row");
	const expectedRequirements = [
		...requirements.filter((requirement) => !requirement.satisfied),
		...requirements.filter((requirement) => requirement.satisfied),
	].flatMap((requirement) => {
		const label = secretRequirementLabel(requirement);
		return label ? [{ requirement, label }] : [];
	});
	const missingCount = expectedRequirements.filter(
		({ requirement }) => !requirement.satisfied,
	).length;

	expect(table).toBeVisible();
	expect(rows).toHaveLength(expectedRequirements.length + 1);
	expect(rows[0]).toHaveTextContent(/secret/i);
	expect(rows[0]).toHaveTextContent(/description/i);
	expect(rows[0]).toHaveTextContent(/action/i);
	const bodyRows = rows.slice(1);
	for (const [index, { label }] of expectedRequirements.entries()) {
		expect(bodyRows[index]).toHaveTextContent(label);
	}

	const manageSecretsLink = canvas.getByRole("link", {
		name: /view cli documentation/i,
	});
	expect(manageSecretsLink).toBeVisible();
	expect(manageSecretsLink).toHaveAttribute(
		"href",
		expect.stringContaining("/reference/cli/secret_create"),
	);
	expect(manageSecretsLink).toHaveAttribute("target", "_blank");

	if (missingCount >= 1) {
		expect(canvas.getByRole("status")).toHaveTextContent(
			missingCount === 1
				? "1 required secret is missing."
				: `${missingCount} required secrets are missing.`,
		);

		const addSecretLinks = canvas.getAllByRole("link", { name: "Add Secret" });
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

	expect(canvas.queryByRole("status")).toBeNull();
	expect(canvas.queryByRole("link", { name: "Add Secret" })).toBeNull();
};

const githubTokenHelpMessage = "Add a GitHub PAT with env=GITHUB_TOKEN";

const githubToken: SecretRequirementStatus = {
	env: "GITHUB_TOKEN",
	help_message: githubTokenHelpMessage,
	satisfied: false,
};

const awsCredentials: SecretRequirementStatus = {
	file: "~/.aws/credentials",
	help_message: "Add AWS credentials file",
	satisfied: true,
};

export const Empty: Story = {
	args: {
		requirements: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByRole("table", { name: /required secrets/i }),
		).toBeNull();
	},
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

export const TooltipOnlyWhenTruncated: Story = {
	args: {
		requirements: [githubToken, awsCredentials],
	},
	render: (args) => (
		<div style={{ width: 640 }}>
			<SecretsTable {...args} />
		</div>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const githubTooltipButton = await canvas.findByRole("button", {
			name: /show full description for github_token/i,
		});
		expect(githubTooltipButton).toBeVisible();
		expect(
			canvas.queryByRole("button", {
				name: /show full description for ~\/\.aws\/credentials/i,
			}),
		).toBeNull();

		await userEvent.hover(githubTooltipButton);
		await waitFor(() =>
			expect(
				within(document.body).getByText(githubTokenHelpMessage),
			).toBeVisible(),
		);
		await userEvent.unhover(githubTooltipButton);
	},
};
