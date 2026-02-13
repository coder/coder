import { API } from "api/api";
import type { UserDiagnosticResponse } from "pages/OperatorDiagnosticPage/types";

export function userDiagnostic(
	username: string,
	hours = 72,
	status = "all",
	workspace = "all",
) {
	return {
		queryKey: ["userDiagnostic", username, hours, status, workspace] as const,
		queryFn: (): Promise<UserDiagnosticResponse> =>
			API.getUserDiagnostic(username, hours, status, workspace),
	};
}
