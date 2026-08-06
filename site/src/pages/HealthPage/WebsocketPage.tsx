import { CodeIcon } from "lucide-react";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { pageTitle } from "#/utils/page";
import {
	Header,
	HeaderTitle,
	HealthyDot,
	Main,
	Pill,
	SectionLabel,
} from "./Content";
import { MuteWarningsButton } from "./MuteWarningsButton";
import { useHealthStatus } from "./useHealthStatus";

const WebsocketPage = () => {
	const { data: healthStatus, isLoading, error } = useHealthStatus();

	if (isLoading) {
		return <Loader />;
	}

	if (error || !healthStatus) {
		return <ErrorAlert error={error} />;
	}

	const { websocket } = healthStatus;

	return (
		<>
			<title>{pageTitle("Websocket - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={websocket.severity} />
					Websocket
				</HeaderTitle>
				<MuteWarningsButton healthcheck="Websocket" />
			</Header>

			<Main>
				{websocket.error && (
					<Alert severity="error" prominent>
						{websocket.error}
					</Alert>
				)}

				{websocket.warnings.map((warning) => {
					return (
						<Alert key={warning.code} severity="warning" prominent dismissible>
							{warning.message}
						</Alert>
					);
				})}

				<section>
					<Tooltip>
						<TooltipTrigger asChild>
							<Pill icon={<CodeIcon className="size-icon-sm" />}>
								{websocket.code}
							</Pill>
						</TooltipTrigger>
						<TooltipContent side="bottom">Code</TooltipContent>
					</Tooltip>
				</section>

				<section>
					<SectionLabel>Body</SectionLabel>
					<div className="bg-surface-secondary border border-solid border-border rounded-lg text-sm p-6 font-mono">
						{websocket.body !== "" ? (
							websocket.body
						) : (
							<span className="text-content-secondary">No body message</span>
						)}
					</div>
				</section>
			</Main>
		</>
	);
};

export default WebsocketPage;
