import { createMachine } from 'xstate';

/** If using VSCode, install the XState VSCode extension to get a 
 * "Open Inspector" link above each machine in your code that will 
 * visualize the machine. Otherwise, you can paste code into stately.ai/viz.
 */

interface WorkspaceContext {
  errorMessage?: string
}

type WorkspaceEvent =
  | { type: "START" }
  | { type: "STOP" }
  | { type: "REBUILD" }

export const workspaceModel = createMachine<WorkspaceContext, WorkspaceEvent>(
  {
    id: "workspaceV2Model",
    initial: "off",
    states: {
      off: { 
        on: {
          START: "starting"
        }
      },
      starting: {
        invoke: {
          src: "buildWorkspace",
          onDone: "running",
          onError: "error",
        },
      },
      running: {
        on: {
          STOP: "stopping",
          REBUILD: "rebuilding"
        }
      },
      stopping: {
        invoke: {
          src: "stopWorkspace",
          onDone: "off",
          onError: "error"
        }
      },
      rebuilding: {
        invoke: {
          src: "stopAndStartWorkspace",
          onDone: "running",
          onError: "error"
        }
      },
      error: {
        entry: "saveErrorMessage",
      },
    },
  },
)

export const provisionerModel = createMachine({
  id: 'provisionerModel',
  initial: 'starting',
  context: {
    automator: undefined,
    supportedProjects: []
  },
  states: {
    starting: {
      on: {
        PROVISION: 'provisioning',
        PARSE: 'parsing'
      }
    },
    provisioning: {
      invoke: {
        src: "provision",
        onDone: 'off',
        onError: 'off'
      }
    },
    parsing: {
      invoke: {
        src: 'parse',
        onDone: 'off',
        onError: 'off'
      }
    },
    off: {
      type: 'final'
    }
  }
})

export const provisionerDaemonModel = createMachine({
  id: 'provisionerd',
  initial: 'polling',
  states: {
    polling: {
      on: { 
        JOB_HAS_PROVISIONER_REGISTERED: 'executing',
        NO_JOB_READY: 'polling'
      }
    },
    executing: {
      invoke: {
        id: 'callProvisionerWithParameters',
        src: 'provisionerModel',
        onDone: {target: 'polling', actions: ['returnState', 'parseProjectCode']},
        onError: {target: 'polling', actions: ['returnState', 'markJobFailed']}
      }
    }
  }
}, {
  services: { provisionerModel }
})