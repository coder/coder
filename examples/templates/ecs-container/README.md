---
name: Develop in an ECS-hosted container
description: Get started with Linux development on AWS ECS.
tags: [cloud, aws]
---

# aws-ecs

This is a sample template for running a Coder workspace on ECS.

## Required permissions / policy

The following sample policy allows Coder to create EC2 instances and modify
instances provisioned by Coder:

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
```

Additionally, the `AmazonEC2ContainerServiceforEC2Role` managed policy should be
attached to the container instance IAM role, otherwise you will receive an error
when creating the ECS cluster.

This is represented as the `iam_instance_role` argument of the `launch_template`
resource. Please see the [AWS documentation for configuring this instance role](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/instance_IAM_role.html#instance-iam-role-verify).

## Architecture

This workspace is built using the following AWS resources:

- Launch template - this defines the EC2 instance(s) to host the container
- Auto-scaling group - EC2 auto-scaling group configuration
- ECS cluster - logical grouping of containers to be run in ECS
- Capacity provider - ECS-specific resource that ties in the auto-scaling group
- Task definition - the container definition, includes the image, command, volume(s)
- ECS service - manages the task definition

## User data

This template includes a two-part user data configuration, represented as the
`cloudinit_config` data source. There is an ECS-specific user data definition,
which is required for the EC2 instances to join the ECS cluster. Additionally, the
Coder user data (defined in the `locals` block) is needed to stop/start the instance(s).

## code-server

`code-server` is installed via the `startup_script` argument in the `coder_agent`
resource block. The `coder_app` resource is defined to access `code-server` through
the dashboard UI over `localhost:13337`.
