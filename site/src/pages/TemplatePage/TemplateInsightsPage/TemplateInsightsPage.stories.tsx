import type { Meta, StoryObj } from "@storybook/react";
import { TemplateInsightsPageView } from "./TemplateInsightsPage";

const meta: Meta<typeof TemplateInsightsPageView> = {
  title: "pages/TemplateInsightsPageView",
  component: TemplateInsightsPageView,
};

export default meta;
type Story = StoryObj<typeof TemplateInsightsPageView>;

export const Loading: Story = {
  args: {
    templateInsights: undefined,
    userLatency: undefined,
  },
};

export const Empty: Story = {
  args: {
    templateInsights: {
      interval_reports: [],
      report: {
        active_users: 0,
        end_time: "",
        start_time: "",
        template_ids: [],
        apps_usage: [],
        parameters_usage: [],
      },
    },
    userLatency: {
      report: {
        end_time: "",
        start_time: "",
        template_ids: [],
        users: [],
      },
    },
  },
};

export const Loaded: Story = {
  args: {
    // Got from dev.coder.com network calls
    templateInsights: {
      report: {
        start_time: "2023-07-18T00:00:00Z",
        end_time: "2023-07-25T00:00:00Z",
        template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
        active_users: 14,
        apps_usage: [
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            type: "builtin",
            display_name: "Visual Studio Code",
            slug: "vscode",
            icon: "/icon/code.svg",
            seconds: 2513400,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            type: "builtin",
            display_name: "JetBrains",
            slug: "jetbrains",
            icon: "/icon/intellij.svg",
            seconds: 0,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            type: "builtin",
            display_name: "Web Terminal",
            slug: "reconnecting-pty",
            icon: "/icon/terminal.svg",
            seconds: 110400,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            type: "builtin",
            display_name: "SSH",
            slug: "ssh",
            icon: "/icon/terminal.svg",
            seconds: 1020900,
          },
        ],
        parameters_usage: [
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "",
            name: "Compute instances",
            type: "number",
            description: "Let's set the expected number of instances.",
            values: [
              {
                value: "3",
                count: 2,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "",
            name: "Docker Image",
            type: "string",
            description: "Docker image for the development container",
            values: [
              {
                value: "ghcr.io/harrison-ai/coder-dev:base",
                count: 2,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "Very random string",
            name: "Optional random string",
            type: "string",
            description: "This string is optional",
            values: [
              {
                value: "ksjdlkajs;djálskd'l ;a k;aosdk ;oaids ;li",
                count: 1,
              },
              {
                value: "some other any string here",
                count: 1,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "",
            name: "Region",
            type: "string",
            description: "These are options.",
            options: [
              {
                name: "US Central",
                description: "Select for central!",
                value: "us-central1-a",
                icon: "/icon/goland.svg",
              },
              {
                name: "US East",
                description: "Select for east!",
                value: "us-east1-a",
                icon: "/icon/folder.svg",
              },
              {
                name: "US West",
                description: "Select for west!",
                value: "us-west2-a",
                icon: "",
              },
            ],
            values: [
              {
                value: "us-central1-a",
                count: 1,
              },
              {
                value: "us-west2-a",
                count: 1,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "",
            name: "Security groups",
            type: "list(string)",
            description: "Select appropriate security groups.",
            values: [
              {
                value:
                  '["Web Server Security Group","Database Security Group","Backend Security Group"]',
                count: 2,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "Very random string",
            name: "buggy-1",
            type: "string",
            description: "This string is buggy",
            values: [
              {
                value: "",
                count: 2,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "Force rebuild",
            name: "force-rebuild",
            type: "bool",
            description: "Rebuild the project code",
            values: [
              {
                value: "false",
                count: 2,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "Location",
            name: "location",
            type: "string",
            description: "What location should your workspace live in?",
            options: [
              {
                name: "US (Virginia)",
                description: "",
                value: "eastus",
                icon: "/emojis/1f1fa-1f1f8.png",
              },
              {
                name: "US (Virginia) 2",
                description: "",
                value: "eastus2",
                icon: "/emojis/1f1fa-1f1f8.png",
              },
              {
                name: "US (Texas)",
                description: "",
                value: "southcentralus",
                icon: "/emojis/1f1fa-1f1f8.png",
              },
              {
                name: "US (Washington)",
                description: "",
                value: "westus2",
                icon: "/emojis/1f1fa-1f1f8.png",
              },
              {
                name: "US (Arizona)",
                description: "",
                value: "westus3",
                icon: "/emojis/1f1fa-1f1f8.png",
              },
              {
                name: "US (Iowa)",
                description: "",
                value: "centralus",
                icon: "/emojis/1f1fa-1f1f8.png",
              },
              {
                name: "Canada (Toronto)",
                description: "",
                value: "canadacentral",
                icon: "/emojis/1f1e8-1f1e6.png",
              },
              {
                name: "Brazil (Sao Paulo)",
                description: "",
                value: "brazilsouth",
                icon: "/emojis/1f1e7-1f1f7.png",
              },
              {
                name: "East Asia (Hong Kong)",
                description: "",
                value: "eastasia",
                icon: "/emojis/1f1f0-1f1f7.png",
              },
              {
                name: "Southeast Asia (Singapore)",
                description: "",
                value: "southeastasia",
                icon: "/emojis/1f1f0-1f1f7.png",
              },
              {
                name: "Australia (New South Wales)",
                description: "",
                value: "australiaeast",
                icon: "/emojis/1f1e6-1f1fa.png",
              },
              {
                name: "China (Hebei)",
                description: "",
                value: "chinanorth3",
                icon: "/emojis/1f1e8-1f1f3.png",
              },
              {
                name: "India (Pune)",
                description: "",
                value: "centralindia",
                icon: "/emojis/1f1ee-1f1f3.png",
              },
              {
                name: "Japan (Tokyo)",
                description: "",
                value: "japaneast",
                icon: "/emojis/1f1ef-1f1f5.png",
              },
              {
                name: "Korea (Seoul)",
                description: "",
                value: "koreacentral",
                icon: "/emojis/1f1f0-1f1f7.png",
              },
              {
                name: "Europe (Ireland)",
                description: "",
                value: "northeurope",
                icon: "/emojis/1f1ea-1f1fa.png",
              },
              {
                name: "Europe (Netherlands)",
                description: "",
                value: "westeurope",
                icon: "/emojis/1f1ea-1f1fa.png",
              },
              {
                name: "France (Paris)",
                description: "",
                value: "francecentral",
                icon: "/emojis/1f1eb-1f1f7.png",
              },
              {
                name: "Germany (Frankfurt)",
                description: "",
                value: "germanywestcentral",
                icon: "/emojis/1f1e9-1f1ea.png",
              },
              {
                name: "Norway (Oslo)",
                description: "",
                value: "norwayeast",
                icon: "/emojis/1f1f3-1f1f4.png",
              },
              {
                name: "Sweden (Gävle)",
                description: "",
                value: "swedencentral",
                icon: "/emojis/1f1f8-1f1ea.png",
              },
              {
                name: "Switzerland (Zurich)",
                description: "",
                value: "switzerlandnorth",
                icon: "/emojis/1f1e8-1f1ed.png",
              },
              {
                name: "Qatar (Doha)",
                description: "",
                value: "qatarcentral",
                icon: "/emojis/1f1f6-1f1e6.png",
              },
              {
                name: "UAE (Dubai)",
                description: "",
                value: "uaenorth",
                icon: "/emojis/1f1e6-1f1ea.png",
              },
              {
                name: "South Africa (Johannesburg)",
                description: "",
                value: "southafricanorth",
                icon: "/emojis/1f1ff-1f1e6.png",
              },
              {
                name: "UK (London)",
                description: "",
                value: "uksouth",
                icon: "/emojis/1f1ec-1f1e7.png",
              },
            ],
            values: [
              {
                value: "brazilsouth",
                count: 1,
              },
              {
                value: "switzerlandnorth",
                count: 1,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "",
            name: "mtojek_region",
            type: "string",
            description: "What region should your workspace live in?",
            options: [
              {
                name: "Los Angeles, CA",
                description: "",
                value: "Los Angeles, CA",
                icon: "",
              },
              {
                name: "Moncks Corner, SC",
                description: "",
                value: "Moncks Corner, SC",
                icon: "",
              },
              {
                name: "Eemshaven, NL",
                description: "",
                value: "Eemshaven, NL",
                icon: "",
              },
            ],
            values: [
              {
                value: "Los Angeles, CA",
                count: 2,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "My Project ID",
            name: "project_id",
            type: "string",
            description: "This is the Project ID.",
            values: [
              {
                value: "12345",
                count: 2,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "Force devcontainer rebuild",
            name: "rebuild_devcontainer",
            type: "bool",
            description: "",
            values: [
              {
                value: "false",
                count: 2,
              },
            ],
          },
          {
            template_ids: ["7dd1d090-3e23-4ada-8894-3945affcad42"],
            display_name: "Git Repo URL",
            name: "repo_url",
            type: "string",
            description:
              "See sample projects (https://github.com/microsoft/vscode-dev-containers#sample-projects)",
            values: [
              {
                value: "https://github.com/mtojek/coder",
                count: 2,
              },
            ],
          },
        ],
      },
      interval_reports: [
        {
          start_time: "2023-07-18T00:00:00Z",
          end_time: "2023-07-19T00:00:00Z",
          template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
          interval: "day",
          active_users: 13,
        },
        {
          start_time: "2023-07-19T00:00:00Z",
          end_time: "2023-07-20T00:00:00Z",
          template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
          interval: "day",
          active_users: 11,
        },
        {
          start_time: "2023-07-20T00:00:00Z",
          end_time: "2023-07-21T00:00:00Z",
          template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
          interval: "day",
          active_users: 11,
        },
        {
          start_time: "2023-07-21T00:00:00Z",
          end_time: "2023-07-22T00:00:00Z",
          template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
          interval: "day",
          active_users: 13,
        },
        {
          start_time: "2023-07-22T00:00:00Z",
          end_time: "2023-07-23T00:00:00Z",
          template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
          interval: "day",
          active_users: 7,
        },
        {
          start_time: "2023-07-23T00:00:00Z",
          end_time: "2023-07-24T00:00:00Z",
          template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
          interval: "day",
          active_users: 5,
        },
        {
          start_time: "2023-07-24T00:00:00Z",
          end_time: "2023-07-25T00:00:00Z",
          template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
          interval: "day",
          active_users: 11,
        },
      ],
    },
    userLatency: {
      report: {
        start_time: "2023-07-18T00:00:00Z",
        end_time: "2023-07-25T00:00:00Z",
        template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
        users: [
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "0bac0dfd-b086-4b6d-b8ba-789e0eca7451",
            username: "kylecarbs",
            avatar_url: "https://avatars.githubusercontent.com/u/7122116?v=4",
            latency_ms: {
              p50: 63.826,
              p95: 139.328,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "12b03f43-1bb7-4fca-967a-585c97f31682",
            username: "coadler",
            avatar_url: "https://avatars.githubusercontent.com/u/6332295?v=4",
            latency_ms: {
              p50: 51.0745,
              p95: 54.62562499999999,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "15890ddb-142c-443d-8fd5-cd8307256ab1",
            username: "jsjoeio",
            avatar_url: "https://avatars.githubusercontent.com/u/3806031?v=4",
            latency_ms: {
              p50: 37.444,
              p95: 37.8488,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "3f8c0eef-6a45-4759-a4d6-d00bbffb1369",
            username: "dean",
            avatar_url: "https://avatars.githubusercontent.com/u/11241812?v=4",
            latency_ms: {
              p50: 7.1295,
              p95: 70.34084999999999,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "59da0bfe-9c99-47fa-a563-f9fdb18449d0",
            username: "cian",
            avatar_url:
              "https://lh3.googleusercontent.com/a/AAcHTtdsYrtIfkXU52rHXhY9DHehpw-slUKe9v6UELLJgXT2mDM=s96-c",
            latency_ms: {
              p50: 42.14975,
              p95: 125.5441,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "5ccd3128-cbbb-4cfb-8139-5a1edbb60c71",
            username: "bpmct",
            avatar_url: "https://avatars.githubusercontent.com/u/22407953?v=4",
            latency_ms: {
              p50: 42.175,
              p95: 43.437599999999996,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "631f78f6-098e-4cb0-ae4f-418fafb0a406",
            username: "matifali",
            avatar_url: "https://avatars.githubusercontent.com/u/10648092?v=4",
            latency_ms: {
              p50: 78.02,
              p95: 86.3328,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "740bba7f-356d-4203-8f15-03ddee381998",
            username: "eric",
            avatar_url: "https://avatars.githubusercontent.com/u/9683576?v=4",
            latency_ms: {
              p50: 34.533,
              p95: 110.52659999999999,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "78dd2361-4a5a-42b0-9ec3-3eea23af1094",
            username: "code-asher",
            avatar_url: "https://avatars.githubusercontent.com/u/45609798?v=4",
            latency_ms: {
              p50: 74.78875,
              p95: 114.80699999999999,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "7f5cc5e9-20ee-48ce-959d-081b3f52273e",
            username: "mafredri",
            avatar_url: "https://avatars.githubusercontent.com/u/147409?v=4",
            latency_ms: {
              p50: 19.2115,
              p95: 96.44249999999992,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "9ed91bb9-db45-4cef-b39c-819856e98c30",
            username: "jon",
            avatar_url:
              "https://lh3.googleusercontent.com/a/AAcHTtddhPxiGYniy6_rFhdAi2C1YwKvDButlCvJ6G-166mG=s96-c",
            latency_ms: {
              p50: 42.0445,
              p95: 133.846,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "a73425d1-53a7-43d3-b6ae-cae9ba59b92b",
            username: "ammar",
            avatar_url: "https://avatars.githubusercontent.com/u/7416144?v=4",
            latency_ms: {
              p50: 49.249,
              p95: 56.773250000000004,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "af657bc3-6949-4b1b-bc2d-d41a40b546a4",
            username: "BrunoQuaresma",
            avatar_url: "https://avatars.githubusercontent.com/u/3165839?v=4",
            latency_ms: {
              p50: 82.97,
              p95: 147.3868,
            },
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "b006209d-fdd2-4716-afb2-104dafb32dfb",
            username: "mtojek",
            avatar_url: "https://avatars.githubusercontent.com/u/14044910?v=4",
            latency_ms: {
              p50: 36.758,
              p95: 101.31679999999983,
            },
          },
        ],
      },
    },
  },
};
