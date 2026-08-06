import { SidebarLayout } from "#/components/Sidebar";
import { AISettingsSidebar } from "#/modules/management/AISettingsSidebar";

const AISettingsLayout = () => {
	return <SidebarLayout sidebar={<AISettingsSidebar />} />;
};

export default AISettingsLayout;
