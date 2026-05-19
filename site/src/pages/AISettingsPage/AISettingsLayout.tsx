import { Suspense } from "react";
import { Outlet } from "react-router";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "#/components/Breadcrumb/Breadcrumb";
import { Loader } from "#/components/Loader/Loader";
import { AISettingsSidebar } from "#/modules/management/AISettingsSidebar";

const AISettingsLayout = () => {
	return (
		<div>
			<Breadcrumb>
				<BreadcrumbList>
					<BreadcrumbItem>
						<BreadcrumbPage>Admin Settings</BreadcrumbPage>
					</BreadcrumbItem>
					<BreadcrumbSeparator />
					<BreadcrumbItem>
						<BreadcrumbPage className="text-content-primary">AI</BreadcrumbPage>
					</BreadcrumbItem>
				</BreadcrumbList>
			</Breadcrumb>
			<div className="h-px border-none bg-border" />
			<section className="px-10 max-w-screen-2xl mx-auto">
				<div className="flex flex-row gap-28 py-10">
					<AISettingsSidebar />
					<div className="grow">
						<Suspense fallback={<Loader />}>
							<Outlet />
						</Suspense>
					</div>
				</div>
			</section>
		</div>
	);
};

export default AISettingsLayout;
