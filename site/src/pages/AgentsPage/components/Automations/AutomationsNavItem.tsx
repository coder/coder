import { ZapIcon } from "lucide-react";
import { useLocation } from "react-router";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { SettingsNavItem } from "../ChatsSidebar/settings/SettingsNavItem";

export const AUTOMATIONS_PATH = "/agents/automations";

type AutomationsNavItemProps = {
	/** Normalized query string the sidebar appends to its own links. */
	readonly locationSearch: string;
};

export const AutomationsNavItem: React.FC<AutomationsNavItemProps> = ({
	locationSearch,
}) => {
	const { experiments } = useDashboard();
	const location = useLocation();
	if (!experiments.includes("chat-automations")) {
		return null;
	}
	return (
		<SettingsNavItem
			icon={ZapIcon}
			label="Automations"
			active={location.pathname.startsWith(AUTOMATIONS_PATH)}
			to={{ pathname: AUTOMATIONS_PATH, search: locationSearch }}
		/>
	);
};
