import { RadioIcon } from "lucide-react";
import { Link, useLocation } from "react-router";
import { Button } from "#/components/Button/Button";
import { SettingsNavItem } from "../ChatsSidebar/settings/SettingsNavItem";
import { AUTOMATIONS_PATH, useAutomationsEnabled } from "./automationsFlag";

type AutomationsNavItemProps = {
	/** Normalized query string the sidebar appends to its own links. */
	readonly locationSearch: string;
};

export const AutomationsNavItem: React.FC<AutomationsNavItemProps> = ({
	locationSearch,
}) => {
	const location = useLocation();
	return useAutomationsEnabled() ? (
		<SettingsNavItem
			icon={RadioIcon}
			label="Automations"
			active={location.pathname.startsWith(AUTOMATIONS_PATH)}
			to={{ pathname: AUTOMATIONS_PATH, search: locationSearch }}
		/>
	) : null;
};

type AutomationsMobileLinkProps = {
	/** Normalized query string the sidebar appends to its own links. */
	readonly locationSearch: string;
};

/** The mobile header's link to the page; the sidebar nav is hidden there. */
export const AutomationsMobileLink: React.FC<AutomationsMobileLinkProps> = ({
	locationSearch,
}) => {
	return useAutomationsEnabled() ? (
		<Button
			asChild
			variant="subtle"
			size="icon"
			aria-label="Automations"
			className="size-7 sm:hidden"
		>
			<Link to={{ pathname: AUTOMATIONS_PATH, search: locationSearch }}>
				<RadioIcon />
			</Link>
		</Button>
	) : null;
};
