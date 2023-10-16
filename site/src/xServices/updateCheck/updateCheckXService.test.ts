import { waitFor } from "@testing-library/react";
import { MockPermissions, MockUpdateCheck } from "testHelpers/entities";
import { interpret } from "xstate";
import {
  clearDismissedVersionOnLocal,
  getDismissedVersionOnLocal,
  saveDismissedVersionOnLocal,
  updateCheckMachine,
} from "./updateCheckXService";

describe("updateCheckMachine", () => {
  beforeEach(() => {
    clearDismissedVersionOnLocal();
  });

  it("is dismissed when does not have permission to see it", () => {
    const machine = updateCheckMachine.withContext({
      permissions: {
        ...MockPermissions,
        viewUpdateCheck: false,
      },
    });

    const updateCheckService = interpret(machine);
    updateCheckService.start();
    expect(updateCheckService.state.matches("dismissed")).toBeTruthy();
  });

  it("is dismissed when it is already using current version", async () => {
    const machine = updateCheckMachine
      .withContext({
        permissions: {
          ...MockPermissions,
          viewUpdateCheck: true,
        },
      })
      .withConfig({
        services: {
          getUpdateCheck: () =>
            Promise.resolve({
              ...MockUpdateCheck,
              current: true,
            }),
        },
      });

    const updateCheckService = interpret(machine);
    updateCheckService.start();

    await waitFor(() => {
      expect(updateCheckService.state.matches("dismissed")).toBeTruthy();
    });
  });

  it("is dismissed when it was dismissed previously", async () => {
    const machine = updateCheckMachine
      .withContext({
        permissions: {
          ...MockPermissions,
          viewUpdateCheck: true,
        },
      })
      .withConfig({
        services: {
          getUpdateCheck: () =>
            Promise.resolve({
              ...MockUpdateCheck,
              current: false,
            }),
        },
      });

    saveDismissedVersionOnLocal(MockUpdateCheck.version);
    const updateCheckService = interpret(machine);
    updateCheckService.start();

    await waitFor(() => {
      expect(updateCheckService.state.matches("dismissed")).toBeTruthy();
    });
  });

  it("shows when has permission and is outdated", async () => {
    const machine = updateCheckMachine
      .withContext({
        permissions: {
          ...MockPermissions,
          viewUpdateCheck: true,
        },
      })
      .withConfig({
        services: {
          getUpdateCheck: () =>
            Promise.resolve({
              ...MockUpdateCheck,
              current: false,
            }),
        },
      });

    const updateCheckService = interpret(machine);
    updateCheckService.start();

    await waitFor(() => {
      expect(updateCheckService.state.matches("show")).toBeTruthy();
    });
  });

  it("it is dismissed when the DISMISS event happens", async () => {
    const machine = updateCheckMachine
      .withContext({
        permissions: {
          ...MockPermissions,
          viewUpdateCheck: true,
        },
      })
      .withConfig({
        services: {
          getUpdateCheck: () =>
            Promise.resolve({
              ...MockUpdateCheck,
              current: false,
            }),
        },
      });

    const updateCheckService = interpret(machine);
    updateCheckService.start();
    await waitFor(() => {
      expect(updateCheckService.state.matches("show")).toBeTruthy();
    });

    updateCheckService.send("DISMISS");
    await waitFor(() => {
      expect(updateCheckService.state.matches("dismissed")).toBeTruthy();
    });
    expect(getDismissedVersionOnLocal()).toEqual(MockUpdateCheck.version);
  });
});
