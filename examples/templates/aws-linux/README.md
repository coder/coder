---
name: Develop in Linux on AWS EC2
description: Get started with Linux development on AWS EC2.
tags: [cloud, aws]
---

# aws-linux

## Getting started

Pick this template in `coder templates init` and follow instructions.

## Authentication

This template assumes that coderd is run in an environment that is authenticated
with AWS. For example, run `aws configure import` to import credentials on the
system and user running coderd.  For other ways to authenticate [consult the
Terraform docs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs#authentication-and-configuration).

## Required permissions / policy

This example policy allows Coder to create EC2 instances and modify instances provisioned by Coder.

```json
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
            "Sid": "CoderResouces",
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
```

