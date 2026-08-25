import { Navigate, useLocation } from "react-router";

export const ModelDefaultsRedirect = () => {
	const location = useLocation();
	return (
		<Navigate
			to={{ pathname: "/ai/settings/coder-agents", search: location.search }}
			replace
		/>
	);
};
