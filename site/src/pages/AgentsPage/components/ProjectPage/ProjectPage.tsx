import { useLocation, useParams } from "react-router";
import type { ChatProject } from "#/api/typesGenerated";
import { LinkTabs, LinkTabsList, TabLink } from "#/components/Tabs/Tabs";
import { buildAgentProjectPath } from "../../utils/navigation";
import { normalizeLocationSearch } from "../ChatsSidebar/locationSearch";
import { AgentRunsSection } from "./AgentRunsSection";
import { ProjectChatsList } from "./ProjectChatsList";
import { ProjectHeaderSection } from "./ProjectHeaderSection";
import { ProjectSettingsTab } from "./ProjectSettingsTab";

const projectTabs = [
	{ id: "chats", label: "Chats" },
	{ id: "settings", label: "Settings" },
	{ id: "run-history", label: "Run history" },
] as const;

type ProjectPageProps = {
	readonly project: ChatProject;
	/** The chat composer, shown on the Chats tab. */
	readonly children: React.ReactNode;
};

export const ProjectPage: React.FC<ProjectPageProps> = ({
	project,
	children,
}) => {
	const location = useLocation();
	const { tab } = useParams<{ tab?: string }>();
	const activeTab = projectTabs.find((t) => t.id === tab)?.id ?? "chats";
	const search = normalizeLocationSearch(location.search);

	return (
		// flex-1 and min-h-0 bound the page to the layout's height at every
		// breakpoint, so a long chat list scrolls here instead of being clipped.
		<div className="order-last flex min-h-0 flex-1 flex-col overflow-auto px-4 pt-8 sm:order-0 sm:px-8">
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-6 pb-12">
				<ProjectHeaderSection key={project.id} project={project} />
				<LinkTabs active={activeTab} variant="pill">
					<LinkTabsList aria-label="Agent sections">
						{projectTabs.map(({ id, label }) => (
							<TabLink
								key={id}
								value={id}
								to={{
									pathname: buildAgentProjectPath({
										projectId: project.id,
										tab: id === "chats" ? undefined : id,
									}),
									search,
								}}
							>
								{label}
							</TabLink>
						))}
					</LinkTabsList>
				</LinkTabs>
				<div className="flex flex-col gap-8">
					{activeTab === "chats" && (
						<>
							{children}
							<ProjectChatsList project={project} />
						</>
					)}
					{activeTab === "settings" && (
						<ProjectSettingsTab key={project.id} project={project} />
					)}
					{activeTab === "run-history" && (
						<>
							<AgentRunsSection project={project} source="automation" />
							<AgentRunsSection project={project} source="delegation" />
						</>
					)}
				</div>
			</div>
		</div>
	);
};
