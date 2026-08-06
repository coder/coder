import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { Provisioner } from "#/modules/provisioners/Provisioner";
import { pageTitle } from "#/utils/page";
import {
	Header,
	HeaderTitle,
	HealthMessageDocsLink,
	HealthyDot,
	Main,
} from "./Content";
import { MuteWarningsButton } from "./MuteWarningsButton";
import { useHealthStatus } from "./useHealthStatus";

const ProvisionerDaemonsPage: FC = () => {
	const { data: healthStatus, isLoading, error } = useHealthStatus();

	if (isLoading) {
		return <Loader />;
	}

	if (error || !healthStatus) {
		return <ErrorAlert error={error} />;
	}

	const { provisioner_daemons: daemons } = healthStatus;

	return (
		<>
			<title>{pageTitle("Provisioner Daemons - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={daemons.severity} />
					Provisioner Daemons
				</HeaderTitle>
				<MuteWarningsButton healthcheck="ProvisionerDaemons" />
			</Header>

			<Main>
				{daemons.error && (
					<Alert severity="error" prominent>
						{daemons.error}
					</Alert>
				)}
				{daemons.warnings.map((warning) => {
					return (
						<Alert
							actions={<HealthMessageDocsLink {...warning} />}
							key={warning.code}
							severity="warning"
							prominent
							dismissible
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
