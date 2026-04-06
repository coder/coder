import { keepPreviousData, useQuery } from "react-query";
import { API } from "#/api/api";
import { useDebouncedValue } from "#/hooks/debounce";

export function usePasswordValidator(value: string) {
	const debouncedValue = useDebouncedValue(value, 500);
	const query = useQuery({
		queryKey: ["validatePassword", debouncedValue],
		queryFn: () => API.validateUserPassword(debouncedValue),
		placeholderData: keepPreviousData,
		enabled: debouncedValue.length > 0,
	});
	return {
		valid: query.data?.valid ?? true,
		details: query.data?.details,
	};
}
