import { updateCheck } from "api/queries/updateCheck";
import { useMemo, useState } from "react";
import { useQuery } from "react-query";

type UpdateCheckState = "show" | "hide";

export const useUpdateCheck = (enabled: boolean) => {
  const [dismissedVersion, setDismissedVersion] = useState(() =>
    getDismissedVersionOnLocal(),
  );
  const updateCheckQuery = useQuery({
    ...updateCheck(),
    enabled,
  });

  const state: UpdateCheckState = useMemo(() => {
    if (!updateCheckQuery.data) {
      return "hide";
    }

    const isNotDismissed = dismissedVersion !== updateCheckQuery.data.version;
    const isOutdated = !updateCheckQuery.data.current;
    return isNotDismissed && isOutdated ? "show" : "hide";
  }, [dismissedVersion, updateCheckQuery.data]);

  const dismiss = () => {
    if (!updateCheckQuery.data) {
      throw new Error(
        "Cannot dismiss update check when there is no update check data",
      );
    }
    setDismissedVersion(updateCheckQuery.data.version);
    saveDismissedVersionOnLocal(updateCheckQuery.data.version);
  };

  return {
    state,
    dismiss,
    data: updateCheckQuery.data,
  };
};

const saveDismissedVersionOnLocal = (version: string): void => {
  window.localStorage.setItem("dismissedVersion", version);
};

const getDismissedVersionOnLocal = (): string | undefined => {
  return localStorage.getItem("dismissedVersion") ?? undefined;
};
