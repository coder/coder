import { Suspense } from "react";
import { Outlet } from "react-router";
import { Loader } from "#/components/Loader/Loader";

const AISettingsLayout = () => {
	return (
		<Suspense fallback={<Loader />}>
			<Outlet />
		</Suspense>
	);
};

export default AISettingsLayout;
