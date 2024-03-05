import type { Meta, StoryObj } from "@storybook/react";
import TerminalPage from "./TerminalPage";
import { AuthProvider } from "contexts/auth/AuthProvider";
import {
  reactRouterOutlet,
  reactRouterParameters,
} from "storybook-addon-react-router-v6";
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
import { getAuthorizationKey } from "api/queries/authCheck";
import { permissionsToCheck } from "contexts/auth/permissions";
import { workspaceByOwnerAndNameKey } from "api/queries/workspaces";
import { Workspace } from "api/typesGenerated";
import { withWebSocket } from "testHelpers/storybook";

const meta: Meta<typeof TerminalPage> = {
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
                agents: [{ ...MockWorkspaceAgent, lifecycle_state: "ready" }],
              },
            ],
          },
        } satisfies Workspace,
      },
      {
        key: getAuthorizationKey({ checks: permissionsToCheck }),
        data: { editWorkspaceProxies: true },
      },
    ],
    webSocket: {
      // Copied and pasted this from browser
      messages: [
        `[H[2J[1m[32m➜  [36mcoder[C[34mgit:([31mbq/refactor-web-term-notifications[34m) [33m✗`,
      ],
    },
  },
  decorators: [
    (Story) => (
      <AuthProvider>
        <Story />
      </AuthProvider>
    ),
    withWebSocket,
  ],
};

export default meta;
type Story = StoryObj<typeof TerminalPage>;

export const Default: Story = {};
