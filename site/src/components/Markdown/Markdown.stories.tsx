import type { Meta, StoryObj } from "@storybook/react-vite";
import { Markdown } from "./Markdown";

const meta: Meta<typeof Markdown> = {
	title: "components/Markdown",
	component: Markdown,
};

export default meta;
type Story = StoryObj<typeof Markdown>;

export const WithCode: Story = {
	args: {
		children: `
  ## Required permissions / policy

  The following sample policy allows Coder to create EC2 instances and modify instances provisioned by Coder:

  \`\`\`json
  {
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": [
                "ec2:GetDefaultCreditSpecification",
                "ec2:DescribeIamInstanceProfileAssociations",
                "ec2:DescribeTags",
                "ec2:CreateTags",
                "ec2:RunInstances",
                "ec2:DescribeInstanceCreditSpecifications",
                "ec2:DescribeImages",
                "ec2:ModifyDefaultCreditSpecification",
                "ec2:DescribeVolumes"
            ],
            "Resource": "*"
        },
        {
            "Sid": "CoderResources",
            "Effect": "Allow",
            "Action": [
                "ec2:DescribeInstances",
                "ec2:DescribeInstanceAttribute",
                "ec2:UnmonitorInstances",
                "ec2:TerminateInstances",
                "ec2:StartInstances",
                "ec2:StopInstances",
                "ec2:DeleteTags",
                "ec2:MonitorInstances",
                "ec2:CreateTags",
                "ec2:RunInstances",
                "ec2:ModifyInstanceAttribute",
                "ec2:ModifyInstanceCreditSpecification"
            ],
            "Resource": "arn:aws:ec2:*:*:instance/*",
            "Condition": {
                "StringEquals": {
                    "aws:ResourceTag/Coder_Provisioned": "true"
                }
            }
        }
    ]
  }
  \`\`\``,
	},
};

export const WithTable: Story = {
	args: {
		children: `
  | heading | b  |  c |  d  |
  | - | :- | -: | :-: |
  | cell 1 | cell 2 | 3 | 4 | `,
	},
};

export const GFMAlerts: Story = {
	args: {
		children: `
> [!NOTE]
> Useful information that users should know, even when skimming content.

> [!TIP]
> Helpful advice for doing things better or more easily.

> [!IMPORTANT]
> Key information users need to know to achieve their goal.

> [!WARNING]
> Urgent info that needs immediate user attention to avoid problems.

> [!CAUTION]
> Advises about risks or negative outcomes of certain actions.
		`,
	},
};

export const GFMAlertsWithLinks: Story = {
	args: {
		children: `
> [!NOTE]
> This template is centrally managed by CI/CD in the [coder/templates](https://github.com/coder/templates) repository.

> [!TIP]
> Check out the [documentation](https://docs.coder.com) for more information.

> [!IMPORTANT]
> Make sure to read the [security guidelines](https://coder.com/security) before proceeding.

> [!WARNING]
> This action may affect your [workspace settings](https://coder.com/settings).

> [!CAUTION]
> Deleting this will remove all data. See [backup guide](https://coder.com/backup) first.
		`,
	},
};