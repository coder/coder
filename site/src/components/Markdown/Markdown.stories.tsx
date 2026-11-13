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

export const WithHeadings: Story = {
	args: {
		children: `
# Heading 1

This template provisions a Docker container with a persistent home volume.

## Heading 2

The container runs the Coder agent and exposes code-server on port 8080.

### Heading 3

Set the CPU and memory parameters when you create the workspace.

#### Heading 4

The home volume survives workspace restarts.

##### Heading 5

Delete the workspace to remove the volume.

###### Heading 6

See the Docker provider documentation for more options.
`,
	},
};

export const WithHeadingsCompact: Story = {
	args: {
		...WithHeadings.args,
		className: "text-xs",
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

export const GFMAlertWithInlineFormatting: Story = {
	args: {
		children: `
> [!IMPORTANT]
> Larger **instances** cost more. Choose based on your workload.
> Test line two
			`,
	},
};
