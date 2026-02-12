import { API } from "api/api";
import type { UserDiagnosticResponse } from "pages/OperatorDiagnosticPage/types";

export function userDiagnostic(username: string, hours = 72) {
	return {
		queryKey: ["userDiagnostic", username, hours] as const,
		queryFn: (): Promise<UserDiagnosticResponse> =>
			API.getUserDiagnostic(username, hours),
	};
}
