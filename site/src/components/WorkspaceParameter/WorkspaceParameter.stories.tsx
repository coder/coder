import { Story } from "@storybook/react"
import { WorkspaceParameter, WorkspaceParameterProps } from "./WorkspaceParameter"

export default {
  title: "components/WorkspaceParameter",
  component: WorkspaceParameter,
}

const Template: Story<WorkspaceParameterProps> = (args) => <WorkspaceParameter {...args} />

export const Region = Template.bind({})
Region.args = {
  templateParameter: {
    name: "Region",
    default_value: "canada",
    description: "Select a location for your workspace to live.",
    icon: "/emojis/1f30e.png",
    mutable: false,
    options: [
      {
        name: "Toronto, Canada",
        description: "",
        icon: "/emojis/1f1e8-1f1e6.png",
        value: "canada",
      },
      {
        name: "Hamina, Finland",
        description: "",
        icon: "/emojis/1f1eb-1f1ee.png",
        value: "finland",
      },
      {
        name: "Warsaw, Poland",
        description: "",
        icon: "/emojis/1f1f5-1f1f1.png",
        value: "poland",
      },
      {
        name: "Madrid, Spain",
        description: "",
        icon: "/emojis/1f1ea-1f1f8.png",
        value: "spain",
      },
      {
        name: "London, England",
        description: "",
        icon: "/emojis/1f1ec-1f1e7.png",
        value: "england",
      },
      {
        name: "Dallas, Texas",
        description: "",
        icon: "/emojis/1f920.png",
        value: "texas",
      },
    ],
    type: "string",
    validation_max: 0,
    validation_min: 0,
    validation_regex: "",
    validation_error: "",
  },
  workspaceBuildParameter: {
    name: "Region",
    value: "canada",
  },
}

export const Repo = Template.bind({})
Repo.args = {
  templateParameter: {
    name: "Repo",
    default_value: "coder",
    description: "Select a repository to work on. This will automatically be cloned.",
    icon: "/icon/github.svg",
    mutable: false,
    options: [
      {
        name: "coder/coder",
        description:
          "Remote development environments on your infrastructure provisioned with Terraform",
        icon: "",
        value: "https://github.com/coder/coder",
      },
      {
        name: "coder/v1",
        description: "The home for Coder v1!",
        icon: "",
        value: "https://github.com/coder/v1",
      },
    ],
    type: "string",
    validation_max: 0,
    validation_min: 0,
    validation_regex: "",
    validation_error: "",
  },
  workspaceBuildParameter: {
    name: "Repo",
    value: "https://github.com/coder/coder",
  },
}

export const Size = Template.bind({})
Size.args = {
  templateParameter: {
    name: "Instance Size",
    default_value: "8",
    description: "",
    icon: "/emojis/1f916.png",
    mutable: true,
    options: [
      {
        name: "Small",
        description: "A tiny 4 core machine for small projects.",
        icon: "/emojis/1f90f.png",
        value: "4",
      },
      {
        name: "Medium",
        description: "A larger 8 core machine for heavy-ish workloads.",
        icon: "/emojis/1f44c.png",
        value: "8",
      },
      {
        name: "Large",
        description: "A beefy 16 core machine that can power most workloads.",
        icon: "/emojis/1f4aa.png",
        value: "16",
      },
    ],
    type: "string",
    validation_max: 0,
    validation_min: 0,
    validation_regex: "",
    validation_error: "",
  },
  workspaceBuildParameter: {
    name: "Instance Size",
    value: "8",
  },
}

export const Dotfiles = Template.bind({})
Dotfiles.args = {
  templateParameter: {
    name: "Dotfiles URL",
    default_value: "https://github.com/ammario/dotfiles",
    description:
      "A Git URL that points to your personal dotfiles! These will be automatically cloned at start.",
    icon: "/emojis/1f3a8.png",
    mutable: true,
    type: "string",
    options: [],
    validation_max: 0,
    validation_min: 0,
    validation_regex: "((git|ssh|http(s)?)|(git@[w.]+))(:(//)?)([w.@:/-~]+)(/)?",
    validation_error: "Must be a valid Git URL!",
  },
  workspaceBuildParameter: {
    name: "Dotfiles URL",
    value: "",
  },
}

export const DiskSize = Template.bind({})
DiskSize.args = {
  templateParameter: {
    name: "Disk Size",
    default_value: "10",
    description: "The number of gigabytes for your persistent home volume.",
    icon: "",
    mutable: true,
    type: "number",
    options: [],
    validation_max: 200,
    validation_min: 10,
    validation_regex: "",
    validation_error: "Some GB",
  },
  workspaceBuildParameter: {
    name: "Dotfiles URL",
    value: "",
  },
}
