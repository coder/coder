import { Alert } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import { linkToTemplate, useLinks } from "modules/navigation";
import type { FC } from "react";

interface ClassicParameterFlowDeprecationWarningProps {
	organizationName: string;
	templateName: string;
	isEnabled: boolean;
}

export const ClassicParameterFlowDeprecationWarning: FC<
	ClassicParameterFlowDeprecationWarningProps
> = ({ organizationName, templateName, isEnabled }) => {
	const getLink = useLinks();

	if (!isEnabled) {
		return null;
	}

	const templateSettingsLink = `${getLink(
		linkToTemplate(organizationName, templateName),
	)}/settings`;

	return (
		<Alert severity="warning" className="mb-2">
			<div>
				This template is using the classic parameter flow, which will be{" "}
				<strong>deprecated</strong> in a future release. Please migrate to
				dynamic improved functionality.
			</div>
			<Link className="text-xs" href={templateSettingsLink}>
				Go to Template Settings
			</Link>
		</Alert>
	);
};
