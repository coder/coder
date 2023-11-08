import { putWorkspaceExtension } from "api/api";
import { Workspace } from "api/typesGenerated";
import dayjs from "dayjs";
import { getDeadline, getMaxDeadline, getMinDeadline } from "utils/schedule";
import minMax from "dayjs/plugin/minMax";
import { useMutation } from "react-query";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { getErrorMessage } from "api/errors";

dayjs.extend(minMax);

const callbacks = {
  onSuccess: () => {
    displaySuccess("Updated workspace shutdown time.");
  },
  onError: (error: unknown) => {
    displayError(
      getErrorMessage(error, "Failed to update workspace shutdown time."),
    );
  },
};

export const useIncreaseDeadline = (workspace: Workspace) => {
  const mutation = useMutation({
    mutationFn: (hours: number) => increaseDeadline(workspace, hours),
    ...callbacks,
  });
  return mutation.mutate;
};

export const useDecreaseDeadline = (workspace: Workspace) => {
  const mutation = useMutation({
    mutationFn: (hours: number) => decreaseDeadline(workspace, hours),
    ...callbacks,
  });
  return mutation.mutate;
};

const increaseDeadline = (workspace: Workspace, hours: number) => {
  const proposedDeadline = getDeadline(workspace).add(hours, "hours");
  const newDeadline = dayjs.min(proposedDeadline, getMaxDeadline(workspace));

  return putWorkspaceExtension(workspace.id, newDeadline);
};

const decreaseDeadline = (workspace: Workspace, hours: number) => {
  const proposedDeadline = getDeadline(workspace).subtract(hours, "hours");
  const newDeadline = dayjs.max(proposedDeadline, getMinDeadline());
  return putWorkspaceExtension(workspace.id, newDeadline);
};
