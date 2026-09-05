import { useMemo } from "react";
import { useQuery } from "react-query";
import { updateCheck } from "#/api/queries/updateCheck";
import { useStorage } from "#/hooks/useStorage";
import { defineStorageKey, stringCodec } from "#/storage";

const dismissedUpdateVersionStorage = defineStorageKey<string | null>({
	key: "dismissedVersion",
	codec: stringCodec,
	defaultValue: null,
});

export const useUpdateCheck = (enabled: boolean) => {
	const [dismissedVersion, setDismissedVersion] = useStorage(
		dismissedUpdateVersionStorage,
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

	const dismiss = () => {
		if (!updateCheckQuery.data) {
			return;
		}
		setDismissedVersion(updateCheckQuery.data.version);
	};

	return {
		isVisible,
		dismiss,
		data: updateCheckQuery.data,
	};
};
