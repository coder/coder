import type { Organization } from "api/typesGenerated";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { Navigate, Outlet, useLocation, useParams } from "react-router";

export const TemplateRedirectController: FC = () => {
	const { organizations, showOrganizations } = useDashboard();
	const { organization, template } = useParams() as {
		organization?: string;
		template: string;
	};
	const location = useLocation();

	// We redirect templates without an organization to the default organization,
	// as that's likely what any links floating around expect.
	if (showOrganizations && !organization) {
		const extraPath = removePrefix(location.pathname, `/templates/${template}`);

		return (
			<Navigate
				to={`/templates/${getOrganizationNameByDefault(
					organizations,
				)}/${template}${extraPath}${location.search}`}
				replace
			/>
		);
	}

	// `showOrganizations` can only be false when there is a single organization,
	// so it's safe to throw away the organization name.
	if (!showOrganizations && organization) {
		const extraPath = removePrefix(
			location.pathname,
			`/templates/${organization}/${template}`,
		);

		return (
			<Navigate
				to={`/templates/${template}${extraPath}${location.search}`}
				replace
			/>
		);
	}

	return <Outlet />;
};

const getOrganizationNameByDefault = (
	organizations: readonly Organization[],
) => {
	return organizations.find((org) => org.is_default)?.name;
};

// I really hate doing it this way, but React Router does not provide a better way.
const removePrefix = (self: string, prefix: string) =>
	self.startsWith(prefix) ? self.slice(prefix.length) : self;
