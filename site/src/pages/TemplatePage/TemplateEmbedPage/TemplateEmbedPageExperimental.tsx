import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import { API } from "api/api";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
	PreviewParameter,
	Template,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { FormSection, VerticalForm } from "components/Form/Form";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "hooks";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useClipboard } from "hooks/useClipboard";
import { CheckIcon, CopyIcon } from "lucide-react";
import {
	DynamicParameter,
	getInitialParameterValues,
} from "modules/workspaces/DynamicParameter/DynamicParameter";
import { ExperimentalFormContext } from "pages/CreateWorkspacePage/ExperimentalFormContext";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import {
	type FC,
	useCallback,
	useContext,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";

export type ButtonValues = Record<string, string>;

const TemplateEmbedPageExperimental: FC = () => {
	const { template } = useTemplateLayoutContext();
	const { user } = useAuthenticated();

	const [latestResponse, setLatestResponse] =
		useState<DynamicParametersResponse | null>(null);
	const ws = useRef<WebSocket | null>(null);
	const wsResponseId = useRef<number>(-1);
	const initialParamsSentRef = useRef(false);
	const [wsError, setWsError] = useState<Error | null>(null);

	const sendMessage = useCallback((values: Record<string, string>) => {
		const request: DynamicParametersRequest = {
			id: wsResponseId.current + 1,
			inputs: values,
		};
		if (ws.current && ws.current.readyState === WebSocket.OPEN) {
			ws.current.send(JSON.stringify(request));
			wsResponseId.current = wsResponseId.current + 1;
		}
	}, []);

	const sendInitialParameters = useEffectEvent((params: PreviewParameter[]) => {
		if (initialParamsSentRef.current) return;
		if (params.length === 0) return;
		const initial = getInitialParameterValues(params);
		const inputs: Record<string, string> = {};
		for (const param of initial) {
			if (param.name && param.value) {
				inputs[param.name] = param.value;
			}
		}
		if (Object.keys(inputs).length === 0) return;
		sendMessage(inputs);
		initialParamsSentRef.current = true;
	});

	const onMessage = useEffectEvent((response: DynamicParametersResponse) => {
		if (latestResponse && latestResponse.id >= response.id) {
			return;
		}
		if (!initialParamsSentRef.current && response.parameters?.length > 0) {
			sendInitialParameters([...response.parameters]);
		}
		setLatestResponse(response);
	});

	useEffect(() => {
		const socket = API.templateVersionDynamicParameters(
			user.id,
			template.active_version_id,
			{
				onMessage,
				onError: (e) => {
					if (ws.current === socket) {
						setWsError(e);
					}
				},
				onClose: () => {
					if (ws.current === socket) {
						setWsError(
							new Error(
								"Websocket connection for dynamic parameters unexpectedly closed.",
							),
						);
					}
				},
			},
		);
		ws.current = socket;
		return () => {
			socket.close();
		};
	}, [user.id, template.active_version_id, onMessage]);

	const parameters = useMemo(() => {
		return latestResponse?.parameters
			? [...latestResponse.parameters].sort((a, b) => a.order - b.order)
			: [];
	}, [latestResponse?.parameters]);

	return (
		<TemplateEmbedPageViewExperimental
			template={template}
			parameters={parameters}
			sendMessage={sendMessage}
			error={wsError}
		/>
	);
};

interface TemplateEmbedPageViewExperimentalProps {
	template: Template;
	parameters: PreviewParameter[];
	sendMessage: (values: Record<string, string>) => void;
	error?: unknown;
}

const TemplateEmbedPageViewExperimental: FC<
	TemplateEmbedPageViewExperimentalProps
> = ({ template, parameters, sendMessage, error }) => {
	const experimentalFormContext = useContext(ExperimentalFormContext);
	const [buttonValues, setButtonValues] = useState<ButtonValues>();
	const clipboard = useClipboard({
		textToCopy: getClipboardCopyContent(
			template.name,
			template.organization_name,
			buttonValues,
		),
	});

	useEffect(() => {
		if (parameters.length > 0 && !buttonValues) {
			const values: ButtonValues = { mode: "manual" };
			for (const param of getInitialParameterValues(parameters)) {
				values[`param.${param.name}`] = param.value;
			}
			setButtonValues(values);
		}
	}, [parameters, buttonValues]);

	const handleChange = (parameter: PreviewParameter, value: string) => {
		setButtonValues((prev) => ({
			...(prev ?? {}),
			[`param.${parameter.name}`]: value,
		}));
		sendMessage({ [parameter.name]: value });
	};

	return (
		<>
			<Helmet>
				<title>{pageTitle(template.name)}</title>
			</Helmet>
			{!buttonValues ? (
				<Loader />
			) : (
				<div className="flex items-start gap-12">
					<div className="max-w-3xl ">
						{experimentalFormContext && (
							<div className="mb-4">
								<Button
									size="sm"
									variant="outline"
									onClick={experimentalFormContext.toggleOptedOut}
								>
									Go back to the classic template embed flow
								</Button>
							</div>
						)}
						<VerticalForm>
							<FormSection
								title="Creation mode"
								description="By changing the mode to automatic, when the user clicks the button, the workspace will be created automatically instead of showing a form to the user."
							>
								<RadioGroup
									defaultValue={buttonValues.mode}
									onChange={(_, v) => {
										setButtonValues((prev) => ({ ...(prev ?? {}), mode: v }));
									}}
								>
									<FormControlLabel
										value="manual"
										control={<Radio size="small" />}
										label="Manual"
									/>
									<FormControlLabel
										value="auto"
										control={<Radio size="small" />}
										label="Automatic"
									/>
								</RadioGroup>
							</FormSection>
							{parameters.length > 0 && (
								<div
									css={{ display: "flex", flexDirection: "column", gap: 36 }}
								>
									{parameters.map((parameter) => {
										const value = buttonValues[`param.${parameter.name}`] ?? "";
										return (
											<DynamicParameter
												key={parameter.name}
												parameter={parameter}
												value={value}
												onChange={(val) => handleChange(parameter, val)}
												autofill={false}
											/>
										);
									})}
								</div>
							)}

							{Boolean(error) && (
								<p className="text-content-destructive text-sm">
									{String(error)}
								</p>
							)}
						</VerticalForm>
					</div>
					<div className="flex-1 sticky top-16 flex h-[350px] flex-1 items-center justify-center rounded-md bg-surface-secondary p-16 border border-border border-solid">
						<img src="/open-in-coder.svg" alt="Open in Coder button" />
						<div
							css={{
								padding: "48px 16px",
								position: "absolute",
								bottom: 0,
								left: 0,
								display: "flex",
								justifyContent: "center",
								width: "100%",
							}}
						>
							<Button
								css={{ borderRadius: 999 }}
								variant="outline"
								onClick={clipboard.copyToClipboard}
								disabled={clipboard.showCopiedSuccess}
							>
								{clipboard.showCopiedSuccess ? (
									<CheckIcon className="size-icon-sm" />
								) : (
									<CopyIcon className="size-icon-sm" />
								)}
								Copy button code
							</Button>
						</div>
					</div>
				</div>
			)}
		</>
	);
};

function getClipboardCopyContent(
	templateName: string,
	organization: string,
	buttonValues: ButtonValues | undefined,
): string {
	const deploymentUrl = `${window.location.protocol}//${window.location.host}`;
	const createWorkspaceUrl = `${deploymentUrl}/templates/${organization}/${templateName}/workspace`;
	const params = new URLSearchParams(buttonValues);
	const buttonUrl = `${createWorkspaceUrl}?${params.toString()}`;

	return `[![Open in Coder](${deploymentUrl}/open-in-coder.svg)](${buttonUrl})`;
}

export default TemplateEmbedPageExperimental;
