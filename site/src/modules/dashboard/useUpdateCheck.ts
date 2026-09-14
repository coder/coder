import { useCallback, useMemo, useState } from "react";
import { useQuery } from "react-query";
import { updateCheck } from "#/api/queries/updateCheck";

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
		return Boolean(isNotDismissed && isOutdated);
	}, [dismissedVersion, updateCheckQuery.data]);

	// A stable identity keeps consumers (e.g. effect deps) from re-running just
	// because the hook re-rendered.
	const dismiss = useCallback(() => {
		if (!updateCheckQuery.data) {
			return;
		}
		setDismissedVersion(updateCheckQuery.data.version);
		saveDismissedVersionOnLocal(updateCheckQuery.data.version);
	}, [updateCheckQuery.data]);

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
