import { Suspense } from "react";
import { Outlet } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { AISettingsSidebar } from "#/modules/management/AISettingsSidebar";

const AISettingsLayout = () => {
	return (
		<section className="px-10 w-full max-w-screen-2xl mx-auto">
			<div className="flex flex-row gap-28 py-10">
				<AISettingsSidebar />
				<div className="grow">
					<Suspense fallback={<Loader />}>
						<Outlet />
					</Suspense>
				</div>
			</div>
		</section>
	);
};

export default AISettingsLayout;
