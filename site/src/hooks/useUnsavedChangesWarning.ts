import { useEffect } from "react";
import { unstable_usePrompt as usePrompt } from "react-router";

const MESSAGE = "You have unsaved changes. Are you sure you want to leave?";

export const useUnsavedChangesWarning = (isDirty: boolean) => {
	useEffect(() => {
		const onBeforeUnload = (e: BeforeUnloadEvent) => {
			if (isDirty) {
				e.preventDefault();
				return MESSAGE;
			}
		};

		window.addEventListener("beforeunload", onBeforeUnload);

		return () => {
			window.removeEventListener("beforeunload", onBeforeUnload);
		};
	}, [isDirty]);

	usePrompt({
		message: MESSAGE,
		when: isDirty,
	});
};
