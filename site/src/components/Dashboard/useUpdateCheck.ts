import { updateCheck } from "api/queries/updateCheck";
import { useMemo, useState } from "react";
import { useQuery } from "react-query";

export const useUpdateCheck = (enabled: boolean) => {
  const [dismissedVersion, setDismissedVersion] = useState(() =>
    getDismissedVersionOnLocal(),
  );
  const updateCheckQuery = useQuery({
    ...updateCheck(),
    enabled,
  });

  const isVisible: boolean = useMemo(() => {
    if (!updateCheckQuery.data) {
      return false;
    }

    const isNotDismissed = dismissedVersion !== updateCheckQuery.data.version;
    const isOutdated = !updateCheckQuery.data.current;
    return isNotDismissed && isOutdated ? true : false;
  }, [dismissedVersion, updateCheckQuery.data]);

  const dismiss = () => {
    if (!updateCheckQuery.data) {
      return;
    }
    setDismissedVersion(updateCheckQuery.data.version);
    saveDismissedVersionOnLocal(updateCheckQuery.data.version);
  };

  return {
    isVisible,
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
