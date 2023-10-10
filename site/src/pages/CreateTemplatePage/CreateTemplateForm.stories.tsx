import {
  MockTemplate,
  MockTemplateExample,
  MockTemplateVersionVariable1,
  MockTemplateVersionVariable2,
  MockTemplateVersionVariable3,
  MockTemplateVersionVariable4,
  MockTemplateVersionVariable5,
} from "testHelpers/entities";
import { CreateTemplateForm } from "./CreateTemplateForm";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof CreateTemplateForm> = {
  title: "pages/CreateTemplatePage",
  component: CreateTemplateForm,
  args: {
    isSubmitting: false,
    allowDisableEveryoneAccess: true,
  },
};

export default meta;
type Story = StoryObj<typeof CreateTemplateForm>;

export const Upload: Story = {
  args: {
    upload: {
      isUploading: false,
      onRemove: () => {},
      onUpload: () => {},
      file: undefined,
    },
  },
};

export const StarterTemplate: Story = {
  args: {
    starterTemplate: MockTemplateExample,
  },
};

export const DuplicateTemplateWithVariables: Story = {
  args: {
    copiedTemplate: MockTemplate,
    variables: [
      MockTemplateVersionVariable1,
      MockTemplateVersionVariable2,
      MockTemplateVersionVariable3,
      MockTemplateVersionVariable4,
      MockTemplateVersionVariable5,
    ],
  },
};

export const WithJobError: Story = {
  args: {
    copiedTemplate: MockTemplate,
    jobError:
      "template import provision for start: recv import provision: plan terraform: terraform plan: exit status 1",
    logs: [
      {
        id: 461061,
        created_at: "2023-03-06T14:47:32.501Z",
        log_source: "provisioner_daemon",
        log_level: "info",
        stage: "Adding README.md...",
        output: "",
      },
      {
        id: 461062,
        created_at: "2023-03-06T14:47:32.501Z",
        log_source: "provisioner_daemon",
        log_level: "info",
        stage: "Setting up",
        output: "",
      },
      {
        id: 461063,
        created_at: "2023-03-06T14:47:32.528Z",
        log_source: "provisioner_daemon",
        log_level: "info",
        stage: "Parsing template parameters",
        output: "",
      },
      {
        id: 461064,
        created_at: "2023-03-06T14:47:32.552Z",
        log_source: "provisioner_daemon",
        log_level: "info",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 461065,
        created_at: "2023-03-06T14:47:32.633Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 461066,
        created_at: "2023-03-06T14:47:32.633Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "Initializing the backend...",
      },
      {
        id: 461067,
        created_at: "2023-03-06T14:47:32.71Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 461068,
        created_at: "2023-03-06T14:47:32.711Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "Initializing provider plugins...",
      },
      {
        id: 461069,
        created_at: "2023-03-06T14:47:32.712Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: '- Finding coder/coder versions matching "~\u003e 0.6.12"...',
      },
      {
        id: 461070,
        created_at: "2023-03-06T14:47:32.922Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: '- Finding hashicorp/aws versions matching "~\u003e 4.55"...',
      },
      {
        id: 461071,
        created_at: "2023-03-06T14:47:33.132Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "- Installing hashicorp/aws v4.57.0...",
      },
      {
        id: 461072,
        created_at: "2023-03-06T14:47:37.364Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "- Installed hashicorp/aws v4.57.0 (signed by HashiCorp)",
      },
      {
        id: 461073,
        created_at: "2023-03-06T14:47:38.142Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "- Installing coder/coder v0.6.15...",
      },
      {
        id: 461074,
        created_at: "2023-03-06T14:47:39.083Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "- Installed coder/coder v0.6.15 (signed by a HashiCorp partner, key ID 93C75807601AA0EC)",
      },
      {
        id: 461075,
        created_at: "2023-03-06T14:47:39.394Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 461076,
        created_at: "2023-03-06T14:47:39.394Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "Partner and community providers are signed by their developers.",
      },
      {
        id: 461077,
        created_at: "2023-03-06T14:47:39.394Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "If you'd like to know more about provider signing, you can read about it here:",
      },
      {
        id: 461078,
        created_at: "2023-03-06T14:47:39.394Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "https://www.terraform.io/docs/cli/plugins/signing.html",
      },
      {
        id: 461079,
        created_at: "2023-03-06T14:47:39.394Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 461080,
        created_at: "2023-03-06T14:47:39.394Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "Terraform has created a lock file .terraform.lock.hcl to record the provider",
      },
      {
        id: 461081,
        created_at: "2023-03-06T14:47:39.394Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "selections it made above. Include this file in your version control repository",
      },
      {
        id: 461082,
        created_at: "2023-03-06T14:47:39.394Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "so that Terraform can guarantee to make the same selections by default when",
      },
      {
        id: 461083,
        created_at: "2023-03-06T14:47:39.395Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: 'you run "terraform init" in the future.',
      },
      {
        id: 461084,
        created_at: "2023-03-06T14:47:39.395Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 461085,
        created_at: "2023-03-06T14:47:39.395Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "Terraform has been successfully initialized!",
      },
      {
        id: 461086,
        created_at: "2023-03-06T14:47:39.395Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 461087,
        created_at: "2023-03-06T14:47:39.395Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          'You may now begin working with Terraform. Try running "terraform plan" to see',
      },
      {
        id: 461088,
        created_at: "2023-03-06T14:47:39.395Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "any changes that are required for your infrastructure. All Terraform commands",
      },
      {
        id: 461089,
        created_at: "2023-03-06T14:47:39.395Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "should now work.",
      },
      {
        id: 461090,
        created_at: "2023-03-06T14:47:39.397Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 461091,
        created_at: "2023-03-06T14:47:39.397Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "If you ever set or change modules or backend configuration for Terraform,",
      },
      {
        id: 461092,
        created_at: "2023-03-06T14:47:39.397Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "rerun this command to reinitialize your working directory. If you forget, other",
      },
      {
        id: 461093,
        created_at: "2023-03-06T14:47:39.397Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "commands will detect it and remind you to do so if necessary.",
      },
      {
        id: 461094,
        created_at: "2023-03-06T14:47:39.431Z",
        log_source: "provisioner",
        log_level: "info",
        stage: "Detecting persistent resources",
        output: "Terraform 1.1.9",
      },
      {
        id: 461095,
        created_at: "2023-03-06T14:47:43.759Z",
        log_source: "provisioner",
        log_level: "error",
        stage: "Detecting persistent resources",
        output:
          "Error: configuring Terraform AWS Provider: no valid credential sources for Terraform AWS Provider found.\n\nPlease see https://registry.terraform.io/providers/hashicorp/aws\nfor more information about providing credentials.\n\nError: failed to refresh cached credentials, no EC2 IMDS role found, operation error ec2imds: GetMetadata, http response error StatusCode: 404, request to EC2 IMDS failed\n",
      },
      {
        id: 461096,
        created_at: "2023-03-06T14:47:43.759Z",
        log_source: "provisioner",
        log_level: "error",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 461097,
        created_at: "2023-03-06T14:47:43.777Z",
        log_source: "provisioner_daemon",
        log_level: "info",
        stage: "Cleaning Up",
        output: "",
      },
    ],
  },
};
