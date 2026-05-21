import { ArrowUpRightIcon } from "lucide-react";
import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem as SidebarNavItem,
} from "#/components/Sidebar/Sidebar";

const AISettingsSidebarView: React.FC = () => {
	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				<SidebarNavItem href="/ai/settings/governance">
					AI Governance
				</SidebarNavItem>
				<SidebarNavItem href="/ai/settings">Providers</SidebarNavItem>
				<SidebarNavItem href="/agents/settings/agents">
					<div className="flex flex-row items-center gap-1">
						Manage Coder Agents <ArrowUpRightIcon size={16} />
					</div>
				</SidebarNavItem>
			</div>
		</BaseSidebar>
	);
};

export default AISettingsSidebarView;
