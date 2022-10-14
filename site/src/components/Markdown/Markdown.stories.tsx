import { ComponentMeta, Story } from "@storybook/react"
import { Markdown, MarkdownProps } from "./Markdown"

export default {
  title: "components/Markdown",
  component: Markdown,
} as ComponentMeta<typeof Markdown>

const Template: Story<MarkdownProps> = ({ children }) => (
  <Markdown>{children}</Markdown>
)

export const WithCode = Template.bind({})
WithCode.args = {
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
}

export const WithTable = Template.bind({})
WithTable.args = {
  children: `
  | heading | b  |  c |  d  |
  | - | :- | -: | :-: |
  | cell 1 | cell 2 | 3 | 4 | `,
}
