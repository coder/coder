import { CodeIcon } from "lucide-react";
import { useOutletContext } from "react-router";
import type { HealthcheckReport } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
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
import { DismissWarningButton } from "./DismissWarningButton";

const WebsocketPage = () => {
	const healthStatus = useOutletContext<HealthcheckReport>();
	const { websocket } = healthStatus;

	return (
		<>
			<title>{pageTitle("Websocket - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={websocket.severity} />
					Websocket
				</HeaderTitle>
				<DismissWarningButton healthcheck="Websocket" />
			</Header>

			<Main>
				{websocket.error && (
					<Alert severity="error" prominent>
						{websocket.error}
					</Alert>
				)}

				{websocket.warnings.map((warning) => {
					return (
						<Alert key={warning.code} severity="warning" prominent>
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
					<div className="bg-surface-secondary border border-border rounded-lg text-sm p-6 font-mono">
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
