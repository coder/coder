/**
 * @fileoverview workspaceSchedule is an xstate machine backing a form to CRUD
 * an individual workspace's schedule.
 */
import { assign, createMachine } from "xstate";
import * as API from "../../api/api";
import * as TypesGen from "../../api/typesGenerated";

type Permissions = Record<keyof ReturnType<typeof permissionsToCheck>, boolean>;

export interface WorkspaceScheduleContext {
  getWorkspaceError?: unknown;
  /**
   * Each workspace has their own schedule (start and ttl). For this reason, we
   * re-fetch the workspace to ensure we're up-to-date. As a result, this
   * machine is partially influenced by workspaceXService.
   */
  workspace: TypesGen.Workspace;
  template?: TypesGen.Template;
  getTemplateError?: unknown;
  permissions?: Permissions;
  checkPermissionsError?: unknown;
  submitScheduleError?: unknown;
  autostopChanged?: boolean;
  shouldRestartWorkspace?: boolean;
}

export const checks = {
  updateWorkspace: "updateWorkspace",
} as const;

const permissionsToCheck = (workspace: TypesGen.Workspace) =>
  ({
    [checks.updateWorkspace]: {
      object: {
        resource_type: "workspace",
        resource_id: workspace.id,
        owner_id: workspace.owner_id,
      },
      action: "update",
    },
  }) as const;

export type WorkspaceScheduleEvent =
  | {
      type: "SUBMIT_SCHEDULE";
      autostart: TypesGen.UpdateWorkspaceAutostartRequest;
      autostartChanged: boolean;
      ttl: TypesGen.UpdateWorkspaceTTLRequest;
      autostopChanged: boolean;
    }
  | { type: "RESTART_WORKSPACE" }
  | { type: "APPLY_LATER" };

export const workspaceSchedule =
  /** @xstate-layout N4IgpgJg5mDOIC5QHcD2AnA1rADgQwGMwBlAgC0gFcAbEgFzzrAGIBxAUQBUB9AdQHkASgGliABQCCAYXYBtAAwBdRKBypYASzobUAOxUgAHogCMAJgB0AVgAcAThsAWeXZMA2O2YDsdq1YA0IACeiGbyFl428o4AzFYmMT4xbl5mAL5pgWhYuIQk5FS0xAxMFjB02rpQvBjY+ETMEHpgFhq6AG6omC3lNTn1YArKSCBqmtp6BsYI7tbybmZ+9mYO8u6OgSEIMY6OFo5eC34mdqkLNhlZtblEpBQQNPSMPWAVbdXXA8xg6OgYFjhqIwAGYYAC2ZVefTqeSGBjGWh0+hG02SNmsXhMVhWdkcbkcNm8Xk2oRcFgW7hi8h8djcVkuIGyMNuBQeRRKLzeVTEPzBGlgmj0sEazVaHS6LQKBEwPPQfIFSNgcJGCImyNA0wAtOEYrjsa4vD5bMkEiSEG44uTFosCa41lY3AymTd8vdHsVnpCuVBZfLBbphT8-ugAUC6KC5RYpTLefz-UqlPD1IjJijEMkrBE3G4bO5cfN5GYzGadnsDgt5rmnFZHL4nZ88ndCk9Sjh0HAwLo6AAxcHMYgAVQAQgBZACSPGIUgAEuwACIDgAyckTKuTaqmiEzqTMdOzJixJi82OLwUQNic5MxrjxusS5nr-UbrPdHIssEoACM+d6m2yWE0ugtG0nTdO+X4-n+jzKqo65IpuMx7CYF7ZjYMSJLihpeBsZ4zO4Jj7HahomPINaEo4j7Mq6zYeqUH7flolRQFBtAikBYqgS09GQS+tCyCYwyweM8Fpggjg1hYu5UrqZiOCsjjmGaB5uARtYkfq7jUjYjqZIyDYsm67KetxjHvCxLBBv8gIguC4EMXQ5kwaMcGphq6YpBEB4JFYuqxN4Jhmq4bj7CkiReNEpykSYlEuuZtFcWQqDIO8ghwAw6B0HOGh4NQqBQMwgjsMQnASIIPACCI4jSCugnOcJrlGOeMT7LYyExMhClybESmJMFrU2OFDo+OYOlXE+Bk0W+sCJclVSpbA6WZdluX5RIYhiIuACa3CLhInDsIITmqiJbnbG4Oq1gpaxOHmMRKWY2kWPYObnehOwxDFAxxW+lnoGwXB8EIoiSDIR0ueqjXbFYdgWKcVZmCN5x+EpBJPa4ngmLE8jtV4GS6boqAQHABjOl9vEtmASb1RDWqo75GmGr4aFuGampYpYTgI-IUS+Nzhy47ppPPoZFOtBAtBUymNOmOEUQ1nidJWMe1IPazdIWDsXMHseXgxFEAtjVR32euUTHQi6ksbqJXgWPI8y1rJix23ESso3sdvRLrKxmDEjvpIL+nUf+8VekxvpxoqlsnZD6K5nYLhHnJNjYmsZoEjbuu+ASJqdbrn3C5Nnpth2Xa9nKUcNdMuxPdjaFOFS2ErGakTNQjDu3qkJF2PnE3B1NEGmVU5kV9LMzhNDSvzBa1IqZ4OFbLSMMWj7OZoXrhIfQH41B6+xkzSlaV4BlWU5XlI8ISReyYbJ5h2A4cnI7hI2ZtmcSRNpOZxP7huxeTIe-efUSthMyyU8PHO+R4LRKQks4HMmMDhfzWNFLeRs-5vkApTNc1MEK6hAe4c6pE6QDTCCzJ+tZY4nD1qkdCqQBYZCAA */
  createMachine(
    {
      id: "workspaceScheduleState",
      predictableActionArguments: true,
      tsTypes: {} as import("./workspaceScheduleXService.typegen").Typegen0,
      schema: {
        context: {} as WorkspaceScheduleContext,
        events: {} as WorkspaceScheduleEvent,
        services: {} as {
          getTemplate: {
            data: TypesGen.Template;
          };
        },
      },
      initial: "gettingPermissions",
      states: {
        gettingPermissions: {
          entry: "clearGetPermissionsError",
          invoke: {
            src: "checkPermissions",
            id: "checkPermissions",
            onDone: [
              {
                actions: ["assignPermissions"],
                target: "gettingTemplate",
              },
            ],
            onError: [
              {
                actions: "assignGetPermissionsError",
                target: "error",
              },
            ],
          },
          tags: "loading",
        },
        gettingTemplate: {
          entry: "clearGetTemplateError",
          invoke: {
            src: "getTemplate",
            id: "getTemplate",
            onDone: {
              target: "presentForm",
              actions: ["assignTemplate"],
            },
            onError: {
              target: "error",
              actions: ["assignGetTemplateError"],
            },
          },
          tags: "loading",
        },
        presentForm: {
          on: {
            SUBMIT_SCHEDULE: {
              target: "submittingSchedule",
              actions: "assignAutostopChanged",
            },
          },
        },
        submittingSchedule: {
          invoke: {
            src: "submitSchedule",
            id: "submitSchedule",
            onDone: [
              {
                cond: "autostopChanged",
                target: "showingRestartDialog",
              },
              { target: "done" },
            ],
            onError: {
              target: "presentForm",
              actions: ["assignSubmissionError"],
            },
          },
          tags: "loading",
        },
        showingRestartDialog: {
          on: {
            RESTART_WORKSPACE: {
              target: "done",
              actions: "restartWorkspace",
            },
            APPLY_LATER: "done",
          },
        },
        error: {
          type: "final",
        },
        done: {
          type: "final",
        },
      },
    },
    {
      guards: {
        autostopChanged: (context) => Boolean(context.autostopChanged),
      },
      actions: {
        assignSubmissionError: assign({
          submitScheduleError: (_, event) => event.data,
        }),
        assignPermissions: assign({
          // Setting event.data as Permissions to be more stricted. So we know
          // what permissions we asked for.
          permissions: (_, event) => event.data as Permissions,
        }),
        assignGetPermissionsError: assign({
          checkPermissionsError: (_, event) => event.data,
        }),
        assignTemplate: assign({
          template: (_, event) => event.data,
        }),
        assignGetTemplateError: assign({
          getTemplateError: (_, event) => event.data,
        }),
        clearGetTemplateError: assign({
          getTemplateError: (_) => undefined,
        }),
        assignAutostopChanged: assign({
          autostopChanged: (_, event) => event.autostopChanged,
        }),
        clearGetPermissionsError: assign({
          checkPermissionsError: (_) => undefined,
        }),

        // action instead of service because we fire and forget so that the
        // user can return to the workspace page to see the restart
        restartWorkspace: (context) => {
          if (context.workspace && context.template) {
            return API.startWorkspace(
              context.workspace.id,
              context.template.active_version_id,
            );
          }
        },
      },

      services: {
        getTemplate: async (context) => {
          if (context.workspace) {
            return await API.getTemplate(context.workspace.template_id);
          } else {
            throw Error("Can't fetch template without workspace.");
          }
        },
        checkPermissions: async (context) => {
          if (context.workspace) {
            return await API.checkAuthorization({
              checks: permissionsToCheck(context.workspace),
            });
          } else {
            throw Error(
              "Cannot check permissions without both workspace and user id",
            );
          }
        },
        submitSchedule: async (context, event) => {
          if (!context.workspace?.id) {
            // This state is theoretically impossible, but helps TS
            throw new Error("Failed to load workspace.");
          }

          if (event.autostartChanged) {
            await API.putWorkspaceAutostart(
              context.workspace.id,
              event.autostart,
            );
          }
          if (event.autostopChanged) {
            await API.putWorkspaceAutostop(context.workspace.id, event.ttl);
          }
        },
      },
    },
  );
