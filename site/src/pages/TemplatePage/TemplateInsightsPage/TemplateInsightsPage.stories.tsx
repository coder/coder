import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { MockEntitlementsWithUserLimit } from "testHelpers/entities";
import { TemplateInsightsPageView } from "./TemplateInsightsPage";

const meta: Meta<typeof TemplateInsightsPageView> = {
  title: "pages/TemplatePage/TemplateInsightsPageView",
  parameters: { chromatic },
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
    userActivity: {
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
            times_used: 0,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            type: "builtin",
            display_name: "JetBrains",
            slug: "jetbrains",
            icon: "/icon/intellij.svg",
            seconds: 0,
            times_used: 0,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            type: "builtin",
            display_name: "Web Terminal",
            slug: "reconnecting-pty",
            icon: "/icon/terminal.svg",
            seconds: 110400,
            times_used: 0,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            type: "builtin",
            display_name: "SSH",
            slug: "ssh",
            icon: "/icon/terminal.svg",
            seconds: 1020900,
            times_used: 0,
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
              // Test orphan values
              {
                value: "us-west-orphan",
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
          active_users: 16,
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
    userActivity: {
      report: {
        start_time: "2023-09-03T00:00:00-03:00",
        end_time: "2023-10-01T00:00:00-03:00",
        template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
        users: [
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "0bac0dfd-b086-4b6d-b8ba-789e0eca7451",
            username: "kylecarbs",
            avatar_url: "https://avatars.githubusercontent.com/u/7122116?v=4",
            seconds: 671040,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "12b03f43-1bb7-4fca-967a-585c97f31682",
            username: "coadler",
            avatar_url: "https://avatars.githubusercontent.com/u/6332295?v=4",
            seconds: 1487460,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "15890ddb-142c-443d-8fd5-cd8307256ab1",
            username: "jsjoeio",
            avatar_url: "https://avatars.githubusercontent.com/u/3806031?v=4",
            seconds: 6600,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "1c3e3fff-6a0e-4179-9ba3-27f5443e6fce",
            username: "Kira-Pilot",
            avatar_url: "https://avatars.githubusercontent.com/u/19142439?v=4",
            seconds: 195240,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "2e1e7f76-ae77-424a-a209-f35a99731ec9",
            username: "phorcys420",
            avatar_url: "https://avatars.githubusercontent.com/u/57866459?v=4",
            seconds: 16320,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "3f8c0eef-6a45-4759-a4d6-d00bbffb1369",
            username: "dean",
            avatar_url: "https://avatars.githubusercontent.com/u/11241812?v=4",
            seconds: 533520,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "59da0bfe-9c99-47fa-a563-f9fdb18449d0",
            username: "cian",
            avatar_url:
              "https://lh3.googleusercontent.com/a/ACg8ocKKaBWosY_nuQvecIaUPh5RYjxkEN-C8FNGVPlC0Ch2fx0=s96-c",
            seconds: 607080,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "5ccd3128-cbbb-4cfb-8139-5a1edbb60c71",
            username: "bpmct",
            avatar_url: "https://avatars.githubusercontent.com/u/22407953?v=4",
            seconds: 161340,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "631f78f6-098e-4cb0-ae4f-418fafb0a406",
            username: "matifali",
            avatar_url: "https://avatars.githubusercontent.com/u/10648092?v=4",
            seconds: 202500,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "740bba7f-356d-4203-8f15-03ddee381998",
            username: "eric",
            avatar_url: "https://avatars.githubusercontent.com/u/9683576?v=4",
            seconds: 352680,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "78dd2361-4a5a-42b0-9ec3-3eea23af1094",
            username: "code-asher",
            avatar_url: "https://avatars.githubusercontent.com/u/45609798?v=4",
            seconds: 518640,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "7f5cc5e9-20ee-48ce-959d-081b3f52273e",
            username: "mafredri",
            avatar_url: "https://avatars.githubusercontent.com/u/147409?v=4",
            seconds: 218100,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "8b474a55-d414-4b53-a6ba-760f3d4eed7b",
            username: "kirby",
            avatar_url:
              "https://lh3.googleusercontent.com/a-/ALV-UjUHd9l3CaO99BfVlP8L9D9HqKFOUac7zVCA_Bb_2lj0hcPkQvHkMk4HRaMw4b1YF7E-uHnJO-w8sXf3pqRA2EUP9sDvX6ITd2S2YN23kttVCJKTiI-YEIS8eVDfrF8YLqjfKL3PWsxyiPcgtcdfmPiEnlh4mpUMRXZudwtINfk0W3B9KEpwJTpipdlb57HdYO-mD3DEfmwnpZIO_iVjwnpWZZimXH5g15NVregb8VH_vlsW-vHrMsZ1fRGpm6GWnTcWx2rTImz5Qq5dd15MPKYUxc4wpyYImg07eD41ShzHDJhmDaj_n3hjOwFLuyloLBck-t9skQLWf2r7Voq42jVhzJ2-GAv9atC41_ohG1kq8TpCf9ak6S4hE3xMIB4yzDC0VZxl-BlsBHCuKBRTwC-58yTL2GZI31a0Q9PpR720AyiZaOWhX1QOVZmPZey8b8SG7jWTOfzNa9Shf9E0pz3yyIxFx7KSY5Qeye5AmO1au-rXuWr4whXXY6fsn0tnG4nxdyetCiXd0mOmvYHoJuuQFfqYNjdObduRD0yaVZGL-hPFDYH6K-wiedT1y-66jKXcqjVqe0Rwo7YzcVcP-IeV5RGuJ36TEpC1lhi2V-AnG7pmvIn_4AmXfycclrISO10LgQsrx8bxeBW61t9oTFTZCXXBDAd9bLRxndLi_mWYEfOSnWODgfCrapL_GNZsV0tkQ9x-zvlSXQXtze5bg__uAo7CEnZ20yWT5Gr25_NPsH6vyR3hplKn67qBti5_rKzFQ1sVbcuab2BRmF_Al9MTQw-R2gmd0mle9JRr8tyuwCYh82mBrM-dGebXSdqvabws7_WmF5TNwDHHzeeiHq1_6FYB0tBldx3yWk3U8olZ3SiPAe_NRnY0vUKI3ZANOA-IRYxyTAfjShJE0fRMCe70BsqzJj3RDAciqt5IaP2vZQeImjPZLd2NGo-Bbw=s96-c",
            seconds: 543960,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "9ed91bb9-db45-4cef-b39c-819856e98c30",
            username: "jon",
            avatar_url:
              "https://lh3.googleusercontent.com/a/ACg8ocJEE9R4__Pdh40DHGD-3noKezyw-1qo2auV_cb2gxBg=s96-c",
            seconds: 464100,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "a73425d1-53a7-43d3-b6ae-cae9ba59b92b",
            username: "ammar",
            avatar_url: "https://avatars.githubusercontent.com/u/7416144?v=4",
            seconds: 316200,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "af657bc3-6949-4b1b-bc2d-d41a40b546a4",
            username: "BrunoQuaresma",
            avatar_url: "https://avatars.githubusercontent.com/u/3165839?v=4",
            seconds: 329100,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "b006209d-fdd2-4716-afb2-104dafb32dfb",
            username: "mtojek",
            avatar_url: "https://avatars.githubusercontent.com/u/14044910?v=4",
            seconds: 11520,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "b3e1b884-1a5b-44eb-b8b3-423f8eddc503",
            username: "spikecurtis",
            avatar_url: "https://avatars.githubusercontent.com/u/5375600?v=4",
            seconds: 523140,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "baf63990-16a9-472f-b715-64e24c6bcefb",
            username: "atif",
            avatar_url:
              "https://lh3.googleusercontent.com/a-/ALV-UjVWiI2I5XOkxxi5KwAyfzZlfcOSYlMw8dIwJVwg2satTlOaLUy2PXcFcHCYtMg41DImXlB4F7YFIEW-CR_ANiCol7LnHTFomTyeh5N4ZvVQ4rx_sCl3PARywl0-UBW6usVGRVB8CnHve95q4ZDzJA6wJGVvr7gceCpgGe2A2597_KM1L5KIWKr5SAn41AZgQHZc7pgYJtiyKNleDN8LYzmceOtR3GJgFKjMrSOczNLNI3S2TrRPmIBIr_pZFDI3_npKDmQu9fPiVip5RDTAsuP9PdqruNJ4rB0rBae4Gog-RhqUV4L_i01-bJ6aepjH9gqxEkHHkXi7W0ldH8uV2fsQ4Eul78OQp0NrWxx9xZmFseTPK0toiop3EAWnuyp5ikaAnLodtvJ8L3iZXh45LvDv1ADESYPVAeuyHY5eee54O5xy72HABVB_UTE45Zhq086i4zaTNZoObXPrgiU3uNo0EhDQKa2jPNY2oQO0oZa991Oo9zCT9AULz5RP_3GTnfRMgD8ofCKr8Y3dVmSGI0RYOMI5Yqi76sEROCT5LqwAqRTFeGSMIF7-VI9qCctCtZ50n0OVtbFjPCgUGFVN1gZxe2qb66XCQnZOklTaMadj7KvtgIIJFlBSZJLkoPhSyIdiUAOp3VpDn8jOuEI0109YHzEM7l5KFNL-cHxQQyYB9hquld6y6EVRJdro8uVQdwkZ-_Yu4oD70A-WLb-Gi5RLdbB1iFwr99Lg-l4HNDWhh0h1wT5yhn4kgjPMgeTNT7F6fkiteAIvK_jJjVVh-PtKTt48kPv9c7rbc_jCBP70zUQ9X4Xxf9917BPUfvMgLk0gShSaFXxAGTgA7TzRaEsWSi9_DuJ0Q-yQZXwCJ1Y_1VrSF9B2FKsrugotVoC5BORu9tiaWi9jRP6RymM2X0HxsLv0lUFFVjgV0SZnynBNCgqyS02xAs8vEYpw-T7RJg=s96-c",
            seconds: 2040,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "c0240345-f14a-4632-b713-a0f09c2ed927",
            username: "asher-user",
            avatar_url: "",
            seconds: 0,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "c5eb8310-cf4f-444c-b223-0e991f828b40",
            username: "Emyrk",
            avatar_url: "https://avatars.githubusercontent.com/u/5446298?v=4",
            seconds: 24540,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "d96bf761-3f94-46b3-a1da-6316e2e4735d",
            username: "aslilac",
            avatar_url: "https://avatars.githubusercontent.com/u/418348?v=4",
            seconds: 824820,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "db38462b-e63d-4304-87e9-2640eea4a3b8",
            username: "marc",
            avatar_url: "",
            seconds: 120,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "e9b24091-a633-4ba0-9746-ca325a86f0f5",
            username: "michaelsmith",
            avatar_url:
              "https://lh3.googleusercontent.com/a-/ALV-UjXDMd9gEl4aMyM5ENfj4Ruzzn57AETWW6UuNC3Od03Y3AjvrCDhp8iE4I8L0393C_peQF9PZQyVklGCW-FCzODkvyVojUFqafbFi6AtvxjKn59ZyUVtG0ELoDNZOtRQaqUuMNIjtafNQ19LgwYm7LSB47My__oafDZ6jw6Kd_H-qtx19Vh62t3ACoJBHpDrF0BdDxWGBCkUAlC8aJcnqdRqPbKB5WGGcEfwLzrhLc5REN4CuXzm09_ZpU2jdvMUKCBX9H_8j2wcPwtgY0JG0DfIOX_VgTdM6Zy7BLiVQHSjD-uSkwqOEoXvsuKWlEBt74rqjyNDjjM1NyHiUdKpUd26hI2jcro_yrf4Jli7MCf5SjnkGMxQCgrD6-D9bcyBNzXpc1_5mDWrGpSh0X6pVK6GsmuYAc68hfTIHYVs-jB97mls9ClOJ2m51AdOAlizT80Ram2yJ09l-YbTVd4fG3L9FajMsvRhcvwwvN5tGcOk36KcIm0wFy9NQyH09QP3M1Rr2kDn9MzYYuyAZ9Um0tZydrPN9FA59JUytq8GtwnZZVmlZk2X2fXsCgJBv3dCwuF3THqSvL0M3lQa89-slrp2qgSRekiCmbb0-b62T413mOA9KNXcCvct_NN-JAE0b6o7To8B1WW8-AZiFQ2DesSEXL-CWYfqfecs4hoIrSBnQLa3Pm2Q5O-R7R99eRD7H3EqPihl_TiG2s_8gvLUF7ft55hYkV0j-YzTS4nOnUtEAXSqN-JYAd_BTJPJ0kyJLGIScwUQGoNFUQYs5nmlKPepeNpoQYYpQe0zK4ZVYm6fnRXUgv1cWvkD5RuxbBs1kgoVyZrZSNco8apuIjg6sBejRJFre_m0N6emp-Jn5wIkFB1f6IRb7S1aPvCqrqgqI8mTcI6Z-4Z3E3YwiYsn8_zVF9EPa1f1zpzeoppGd_YKaAxLjyOv_nC15bN3eio43A=s96-c",
            seconds: 449820,
          },
          {
            template_ids: ["0d286645-29aa-4eaf-9b52-cc5d2740c90b"],
            user_id: "fdc2dab9-dabd-4980-843f-2e93042db566",
            username: "sharkymark",
            avatar_url: "https://avatars.githubusercontent.com/u/2022166?v=4",
            seconds: 124440,
          },
        ],
      },
    },
  },
};

export const LoadedWithUserLimit: Story = {
  ...Loaded,
  args: {
    ...Loaded.args,
    entitlements: MockEntitlementsWithUserLimit,
  },
};
