import { userDiagnostic } from "api/queries/diagnostics";
import type { FC } from "react";
import { useState } from "react";
import { useQuery } from "react-query";
import { useNavigate, useParams } from "react-router";
import { pageTitle } from "utils/page";
import { OperatorDiagnosticPageView } from "./OperatorDiagnosticPageView";

const OperatorDiagnosticPage: FC = () => {
	const { username = "sarah-chen" } = useParams<{ username: string }>();
	const navigate = useNavigate();
	const [hours, setHours] = useState(72);

	const diagnosticQuery = useQuery(userDiagnostic(username, hours));

	const handleUserSelect = (newUsername: string) => {
		navigate(`/connectionlog/diagnostics/${newUsername}`);
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
			/>
		</>
	);
};

export default OperatorDiagnosticPage;
