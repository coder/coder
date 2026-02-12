import { userDiagnostic } from "api/queries/diagnostics";
import type { FC } from "react";
import { useState } from "react";
import { useQuery } from "react-query";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { getMockDiagnosticData } from "./mockDataService";
import { OperatorDiagnosticPageView } from "./OperatorDiagnosticPageView";

const OperatorDiagnosticPage: FC = () => {
	const { username = "sarah-chen" } = useParams<{ username: string }>();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const isDemo = searchParams.get("demo") === "true";
	const [hours, setHours] = useState(72);

	const diagnosticQuery = useQuery(
		isDemo
			? {
					queryKey: ["userDiagnostic", username, hours] as const,
					queryFn: () => Promise.resolve(getMockDiagnosticData(username)),
				}
			: userDiagnostic(username, hours),
	);

	const handleUserSelect = (newUsername: string) => {
		const params = isDemo ? "?demo=true" : "";
		navigate(`/connectionlog/diagnostics/${newUsername}${params}`);
	};

	return (
		<>
			<title>{pageTitle(`Diagnostics - ${username}`)}</title>
			<OperatorDiagnosticPageView
				data={diagnosticQuery.data}
				isLoading={diagnosticQuery.isLoading}
				username={username}
				onUserSelect={handleUserSelect}
				onTimeWindowChange={setHours}
				selectedHours={hours}
				isDemo={isDemo}
			/>
		</>
	);
};

export default OperatorDiagnosticPage;
