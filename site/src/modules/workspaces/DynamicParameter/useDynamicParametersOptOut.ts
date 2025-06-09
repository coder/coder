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

			// Since the dynamic-parameters experiment was removed, always use classic parameters
			const optedOut = true;

			return {
				templateId,
				optedOut,
			};
		},
	});
};
