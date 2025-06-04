import { useQuery } from "react-query";

export const optOutKey = (id: string): string => `parameters.${id}.optOut`;

interface UseDynamicParametersOptOutOptions {
	templateId: string | undefined;
	templateUsesClassicParameters: boolean | undefined;
	enabled: boolean;
}

export const useDynamicParametersOptOut = ({
	templateId,
	templateUsesClassicParameters,
	enabled,
}: UseDynamicParametersOptOutOptions) => {
	return useQuery({
		enabled: !!templateId && enabled,
		queryKey: ["dynamicParametersOptOut", templateId],
		queryFn: () => {
			if (!templateId) {
				// This should not happen if enabled is working correctly,
				// but as a type guard and sanity check.
				throw new Error("templateId is required");
			}
			const localStorageKey = optOutKey(templateId);
			const storedOptOutString = localStorage.getItem(localStorageKey);

			let optedOut: boolean;

			if (storedOptOutString !== null) {
				optedOut = storedOptOutString === "true";
			} else {
				optedOut = Boolean(templateUsesClassicParameters);
			}

			return {
				templateId,
				optedOut,
			};
		},
	});
};
