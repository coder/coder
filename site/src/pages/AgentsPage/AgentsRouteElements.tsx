import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import { PanelLeftIcon } from "lucide-react";
import type { FC } from "react";
import { NavLink, useOutletContext, useParams } from "react-router";
import { AgentCreateForm } from "./AgentCreateForm";
import type { AgentsOutletContext } from "./AgentsPageView";
import { AnalyticsPageContent } from "./AnalyticsPageContent";
import { ChimeButton } from "./ChimeButton";
import { SettingsPageContent } from "./SettingsPageContent";
import { WebPushButton } from "./WebPushButton";

export const AgentsSettingsRoute: FC = () => {
	const { section } = useParams();
	const ctx = useOutletContext<AgentsOutletContext>();
	return (
		<SettingsPageContent
			activeSection={section ?? "behavior"}
			canManageChatModelConfigs={ctx.isAgentsAdmin}
			canSetSystemPrompt={ctx.isAgentsAdmin}
		/>
	);
};

export const AgentsAnalyticsRoute: FC = () => {
	return <AnalyticsPageContent />;
};

export const AgentsIndexRoute: FC = () => {
	const ctx = useOutletContext<AgentsOutletContext>();
	return (
		<>
			<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
				<NavLink to="/workspaces" className="inline-flex shrink-0 md:hidden">
					{ctx.logoUrl ? (
						<ExternalImage className="h-6" src={ctx.logoUrl} alt="Logo" />
					) : (
						<CoderIcon className="h-6 w-6 fill-content-primary" />
					)}
				</NavLink>
				{ctx.isSidebarCollapsed && (
					<Button
						variant="subtle"
						size="icon"
						onClick={ctx.onExpandSidebar}
						aria-label="Expand sidebar"
						className="hidden h-7 w-7 min-w-0 shrink-0 md:inline-flex"
					>
						<PanelLeftIcon />
					</Button>
				)}
				<div className="flex min-w-0 flex-1 items-center" />
				<div className="flex items-center gap-2">
					<ChimeButton />
					<WebPushButton />
				</div>
			</div>
			<AgentCreateForm
				onCreateChat={ctx.onCreateChat}
				isCreating={ctx.isCreating}
				createError={ctx.createError}
				modelCatalog={ctx.modelCatalog}
				modelOptions={ctx.modelOptions}
				modelConfigs={ctx.modelConfigs}
				isModelCatalogLoading={ctx.isModelCatalogLoading}
				isModelConfigsLoading={ctx.isModelConfigsLoading}
				modelCatalogError={ctx.modelCatalogError}
				onOpenAnalytics={ctx.onOpenAnalytics}
			/>
		</>
	);
};
