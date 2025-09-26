import type { HealthcheckReport } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Provisioner } from "modules/provisioners/Provisioner";
import type { FC } from "react";
import { useOutletContext } from "react-router";
import { pageTitle } from "utils/page";
import {
	Header,
	HeaderTitle,
	HealthMessageDocsLink,
	HealthyDot,
	Main,
} from "./Content";
import { DismissWarningButton } from "./DismissWarningButton";

const ProvisionerDaemonsPage: FC = () => {
	const healthStatus = useOutletContext<HealthcheckReport>();
	const { provisioner_daemons: daemons } = healthStatus;

	return (
		<>
			<title>{pageTitle("Provisioner Daemons - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={daemons.severity} />
					Provisioner Daemons
				</HeaderTitle>
				<DismissWarningButton healthcheck="ProvisionerDaemons" />
			</Header>

			<Main>
				{daemons.error && <Alert severity="error">{daemons.error}</Alert>}
				{daemons.warnings.map((warning) => {
					return (
						<Alert
							actions={<HealthMessageDocsLink {...warning} />}
							key={warning.code}
							severity="warning"
						>
							{warning.message}
						</Alert>
					);
				})}

				{daemons.items.map(({ provisioner_daemon, warnings }) => (
					<Provisioner
						key={provisioner_daemon.id}
						provisioner={provisioner_daemon}
						warnings={warnings}
					/>
				))}
			</Main>
		</>
	);
};

export default ProvisionerDaemonsPage;
