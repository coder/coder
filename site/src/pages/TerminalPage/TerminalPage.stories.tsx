import type { Meta, StoryObj } from "@storybook/react";
import {
  reactRouterOutlet,
  reactRouterParameters,
} from "storybook-addon-react-router-v6";
import { getAuthorizationKey } from "api/queries/authCheck";
import { workspaceByOwnerAndNameKey } from "api/queries/workspaces";
import type { Workspace, WorkspaceAgentLifecycle } from "api/typesGenerated";
import { AuthProvider } from "contexts/auth/AuthProvider";
import { permissionsToCheck } from "contexts/auth/permissions";
import { RequireAuth } from "contexts/auth/RequireAuth";
import {
  MockAppearanceConfig,
  MockAuthMethodsAll,
  MockBuildInfo,
  MockEntitlements,
  MockExperiments,
  MockUser,
  MockWorkspace,
  MockWorkspaceAgent,
} from "testHelpers/entities";
import { withWebSocket } from "testHelpers/storybook";
import TerminalPage from "./TerminalPage";

const createWorkspaceWithAgent = (lifecycle: WorkspaceAgentLifecycle) => {
  return {
    key: workspaceByOwnerAndNameKey(
      MockWorkspace.owner_name,
      MockWorkspace.name,
    ),
    data: {
      ...MockWorkspace,
      latest_build: {
        ...MockWorkspace.latest_build,
        resources: [
          {
            ...MockWorkspace.latest_build.resources[0],
            agents: [{ ...MockWorkspaceAgent, lifecycle_state: lifecycle }],
          },
        ],
      },
    } satisfies Workspace,
  };
};

const meta = {
  title: "pages/Terminal",
  component: RequireAuth,
  parameters: {
    layout: "fullscreen",
    reactRouter: reactRouterParameters({
      location: {
        pathParams: {
          username: `@${MockWorkspace.owner_name}`,
          workspace: MockWorkspace.name,
        },
      },
      routing: reactRouterOutlet(
        {
          path: `/:username/:workspace/terminal`,
        },
        <TerminalPage />,
      ),
    }),
    queries: [
      { key: ["me"], data: MockUser },
      { key: ["authMethods"], data: MockAuthMethodsAll },
      { key: ["hasFirstUser"], data: true },
      { key: ["buildInfo"], data: MockBuildInfo },
      { key: ["entitlements"], data: MockEntitlements },
      { key: ["experiments"], data: MockExperiments },
      { key: ["appearance"], data: MockAppearanceConfig },
      {
        key: getAuthorizationKey({ checks: permissionsToCheck }),
        data: { editWorkspaceProxies: true },
      },
    ],
  },
  decorators: [
    (Story) => (
      <AuthProvider>
        <Story />
      </AuthProvider>
    ),
  ],
} satisfies Meta<typeof TerminalPage>;

export default meta;
type Story = StoryObj<typeof TerminalPage>;

export const Starting: Story = {
  decorators: [withWebSocket],
  parameters: {
    ...meta.parameters,
    webSocket: [
      {
        event: "message",
        // Copied and pasted this from browser
        data: `[H[2J[1m[32m➜  [36mcoder[C[34mgit:([31mbq/refactor-web-term-notifications[34m) [33m✗`,
      },
    ],
    queries: [...meta.parameters.queries, createWorkspaceWithAgent("starting")],
  },
};

export const Ready: Story = {
  decorators: [withWebSocket],
  parameters: {
    ...meta.parameters,
    webSocket: [
      {
        event: "message",
        // Copied and pasted this from browser
        data: `[H[2J[1m[32m➜  [36mcoder[C[34mgit:([31mbq/refactor-web-term-notifications[34m) [33m✗`,
      },
    ],
    queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
  },
};

export const StartError: Story = {
  decorators: [withWebSocket],
  parameters: {
    ...meta.parameters,
    webSocket: [],
    queries: [
      ...meta.parameters.queries,
      createWorkspaceWithAgent("start_error"),
    ],
  },
};

export const ConnectionError: Story = {
  decorators: [withWebSocket],
  parameters: {
    ...meta.parameters,
    webSocket: [
      {
        event: "error",
      },
    ],
    queries: [...meta.parameters.queries, createWorkspaceWithAgent("ready")],
  },
};
