import {
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersionFileTree,
  MockWorkspaceBuildLogs,
  MockWorkspaceResource,
  MockWorkspaceResource2,
  MockWorkspaceResource3,
} from "testHelpers/entities"
import { TemplateVersionEditor } from "./TemplateVersionEditor"
import type { Meta, StoryObj } from "@storybook/react"

const meta: Meta<typeof TemplateVersionEditor> = {
  title: "components/TemplateVersionEditor",
  component: TemplateVersionEditor,
  args: {
    template: MockTemplate,
    templateVersion: MockTemplateVersion,
    defaultFileTree: MockTemplateVersionFileTree,
  },
  parameters: {
    layout: "fullscreen",
  },
}

export default meta
type Story = StoryObj<typeof TemplateVersionEditor>

export const Example: Story = {}

export const Logs = {
  args: {
    buildLogs: MockWorkspaceBuildLogs,
  },
}

export const Resources: Story = {
  args: {
    buildLogs: MockWorkspaceBuildLogs,
    resources: [
      MockWorkspaceResource,
      MockWorkspaceResource2,
      MockWorkspaceResource3,
    ],
  },
}

export const ManyLogs = {
  args: {
    templateVersion: {
      ...MockTemplateVersion,
      job: {
        ...MockTemplateVersion.job,
        error:
          "template import provision for start: terraform plan: exit status 1",
      },
    },
    buildLogs: [
      {
        id: 938494,
        created_at: "2023-08-25T19:07:43.331Z",
        log_source: "provisioner_daemon",
        log_level: "info",
        stage: "Setting up",
        output: "",
      },
      {
        id: 938495,
        created_at: "2023-08-25T19:07:43.331Z",
        log_source: "provisioner_daemon",
        log_level: "info",
        stage: "Parsing template parameters",
        output: "",
      },
      {
        id: 938496,
        created_at: "2023-08-25T19:07:43.339Z",
        log_source: "provisioner_daemon",
        log_level: "info",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 938497,
        created_at: "2023-08-25T19:07:44.15Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "Initializing the backend...",
      },
      {
        id: 938498,
        created_at: "2023-08-25T19:07:44.215Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "Initializing provider plugins...",
      },
      {
        id: 938499,
        created_at: "2023-08-25T19:07:44.216Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: '- Finding coder/coder versions matching "~> 0.11.0"...',
      },
      {
        id: 938500,
        created_at: "2023-08-25T19:07:44.668Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: '- Finding kreuzwerker/docker versions matching "~> 3.0.1"...',
      },
      {
        id: 938501,
        created_at: "2023-08-25T19:07:44.722Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "- Using coder/coder v0.11.1 from the shared cache directory",
      },
      {
        id: 938502,
        created_at: "2023-08-25T19:07:44.857Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "- Using kreuzwerker/docker v3.0.2 from the shared cache directory",
      },
      {
        id: 938503,
        created_at: "2023-08-25T19:07:45.081Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "Terraform has created a lock file .terraform.lock.hcl to record the provider",
      },
      {
        id: 938504,
        created_at: "2023-08-25T19:07:45.081Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "selections it made above. Include this file in your version control repository",
      },
      {
        id: 938505,
        created_at: "2023-08-25T19:07:45.081Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "so that Terraform can guarantee to make the same selections by default when",
      },
      {
        id: 938506,
        created_at: "2023-08-25T19:07:45.082Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: 'you run "terraform init" in the future.',
      },
      {
        id: 938507,
        created_at: "2023-08-25T19:07:45.083Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "Terraform has been successfully initialized!",
      },
      {
        id: 938508,
        created_at: "2023-08-25T19:07:45.084Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          'You may now begin working with Terraform. Try running "terraform plan" to see',
      },
      {
        id: 938509,
        created_at: "2023-08-25T19:07:45.084Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "any changes that are required for your infrastructure. All Terraform commands",
      },
      {
        id: 938510,
        created_at: "2023-08-25T19:07:45.084Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "should now work.",
      },
      {
        id: 938511,
        created_at: "2023-08-25T19:07:45.084Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "If you ever set or change modules or backend configuration for Terraform,",
      },
      {
        id: 938512,
        created_at: "2023-08-25T19:07:45.084Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output:
          "rerun this command to reinitialize your working directory. If you forget, other",
      },
      {
        id: 938513,
        created_at: "2023-08-25T19:07:45.084Z",
        log_source: "provisioner",
        log_level: "debug",
        stage: "Detecting persistent resources",
        output: "commands will detect it and remind you to do so if necessary.",
      },
      {
        id: 938514,
        created_at: "2023-08-25T19:07:45.143Z",
        log_source: "provisioner",
        log_level: "info",
        stage: "Detecting persistent resources",
        output: "Terraform 1.1.9",
      },
      {
        id: 938515,
        created_at: "2023-08-25T19:07:46.297Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output: "Warning: Argument is deprecated",
      },
      {
        id: 938516,
        created_at: "2023-08-25T19:07:46.297Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output: 'on devcontainer-on-docker.tf line 15, in provider "coder":',
      },
      {
        id: 938517,
        created_at: "2023-08-25T19:07:46.297Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output: "  15:   feature_use_managed_variables = true",
      },
      {
        id: 938518,
        created_at: "2023-08-25T19:07:46.297Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 938519,
        created_at: "2023-08-25T19:07:46.297Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output:
          "Terraform variables are now exclusively utilized for template-wide variables after the removal of support for legacy parameters.",
      },
      {
        id: 938520,
        created_at: "2023-08-25T19:07:46.3Z",
        log_source: "provisioner",
        log_level: "error",
        stage: "Detecting persistent resources",
        output: "Error: ephemeral parameter requires the default property",
      },
      {
        id: 938521,
        created_at: "2023-08-25T19:07:46.3Z",
        log_source: "provisioner",
        log_level: "error",
        stage: "Detecting persistent resources",
        output:
          'on devcontainer-on-docker.tf line 27, in data "coder_parameter" "another_one":',
      },
      {
        id: 938522,
        created_at: "2023-08-25T19:07:46.3Z",
        log_source: "provisioner",
        log_level: "error",
        stage: "Detecting persistent resources",
        output: '  27: data "coder_parameter" "another_one" {',
      },
      {
        id: 938523,
        created_at: "2023-08-25T19:07:46.301Z",
        log_source: "provisioner",
        log_level: "error",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 938524,
        created_at: "2023-08-25T19:07:46.301Z",
        log_source: "provisioner",
        log_level: "error",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 938525,
        created_at: "2023-08-25T19:07:46.303Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output: "Warning: Argument is deprecated",
      },
      {
        id: 938526,
        created_at: "2023-08-25T19:07:46.303Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output: 'on devcontainer-on-docker.tf line 15, in provider "coder":',
      },
      {
        id: 938527,
        created_at: "2023-08-25T19:07:46.303Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output: "  15:   feature_use_managed_variables = true",
      },
      {
        id: 938528,
        created_at: "2023-08-25T19:07:46.303Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output: "",
      },
      {
        id: 938529,
        created_at: "2023-08-25T19:07:46.303Z",
        log_source: "provisioner",
        log_level: "warn",
        stage: "Detecting persistent resources",
        output:
          "Terraform variables are now exclusively utilized for template-wide variables after the removal of support for legacy parameters.",
      },
      {
        id: 938530,
        created_at: "2023-08-25T19:07:46.311Z",
        log_source: "provisioner_daemon",
        log_level: "info",
        stage: "Cleaning Up",
        output: "",
      },
    ],
  },
}
