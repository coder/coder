import CheckOutlined from "@mui/icons-material/CheckOutlined";
import FileCopyOutlined from "@mui/icons-material/FileCopyOutlined";
import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import { API } from "api/api";
import { DetailedError } from "api/errors";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
	FriendlyDiagnostic,
	PreviewParameter,
	Template,
	User,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { FormSection } from "components/Form/Form";
import { Loader } from "components/Loader/Loader";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useClipboard } from "hooks/useClipboard";
import {
	Diagnostics,
	DynamicParameter,
} from "modules/workspaces/DynamicParameter/DynamicParameter";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import {
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";

type ButtonValues = Record<string, string>;

const TemplateEmbedPageExperimental: FC = () => {
	const { template } = useTemplateLayoutContext();
	const [latestResponse, setLatestResponse] =
		useState<DynamicParametersResponse | null>(null);
	const wsResponseId = useRef<number>(-1);
	const ws = useRef<WebSocket | null>(null);
	const [wsError, setWsError] = useState<Error | null>(null);

	const { data: authenticatedUser } = useQuery<User>({
		queryKey: ["authenticatedUser"],
		queryFn: () => API.getAuthenticatedUser(),
	});

	const sendMessage = useCallback((formValues: Record<string, string>) => {
		const request: DynamicParametersRequest = {
			id: wsResponseId.current + 1,
			inputs: formValues,
		};
		if (ws.current && ws.current.readyState === WebSocket.OPEN) {
			ws.current.send(JSON.stringify(request));
			wsResponseId.current = wsResponseId.current + 1;
		}
	}, []);

	const onMessage = useEffectEvent((response: DynamicParametersResponse) => {
		if (latestResponse && latestResponse?.id >= response.id) {
			return;
		}

		setLatestResponse(response);
	});

	useEffect(() => {
		if (!template.active_version_id || !authenticatedUser) {
			return;
		}

		const socket = API.templateVersionDynamicParameters(
			authenticatedUser.id,
			template.active_version_id,
			{
				onMessage,
				onError: (error) => {
					setWsError(error);
				},
				onClose: () => {
					setWsError(
						new DetailedError(
							"Websocket connection for dynamic parameters unexpectedly closed.",
							"Refresh the page to reset the form.",
						),
					);
				},
			},
		);

		ws.current = socket;

		return () => {
			socket.close();
		};
	}, [authenticatedUser, template.active_version_id, onMessage]);

	const sortedParams = useMemo(() => {
		if (!latestResponse?.parameters) {
			return [];
		}
		return [...latestResponse.parameters].sort((a, b) => a.order - b.order);
	}, [latestResponse?.parameters]);

	return (
		<>
			<Helmet>
				<title>{pageTitle(template.name)}</title>
			</Helmet>
			<TemplateEmbedPageView
				template={template}
				parameters={sortedParams}
				diagnostics={latestResponse?.diagnostics ?? []}
				error={wsError}
				sendMessage={sendMessage}
			/>
		</>
	);
};

interface TemplateEmbedPageViewProps {
	template: Template;
	parameters: PreviewParameter[];
	diagnostics: readonly FriendlyDiagnostic[];
	error: unknown;
	sendMessage: (message: Record<string, string>) => void;
}

const TemplateEmbedPageView: FC<TemplateEmbedPageViewProps> = ({
	template,
	parameters,
	diagnostics,
	error,
	sendMessage,
}) => {
	const [buttonValues, setButtonValues] = useState<ButtonValues | undefined>();
	const [localParameters, setLocalParameters] = useState<
		Record<string, string>
	>({});

	useEffect(() => {
		if (parameters) {
			const initialInputs: Record<string, string> = {};
			const currentMode = buttonValues?.mode || "manual";
			const initialButtonParamValues: ButtonValues = { mode: currentMode };

			for (const p of parameters) {
				const initialVal = p.value?.valid ? p.value.value : "";
				initialInputs[p.name] = initialVal;
				initialButtonParamValues[`param.${p.name}`] = initialVal;
			}
			setLocalParameters(initialInputs);

			setButtonValues(initialButtonParamValues);
		}
	}, [parameters, buttonValues?.mode]);

	const handleChange = (
		changedParamInfo: PreviewParameter,
		newValue: string,
	) => {
		const newFormInputs = {
			...localParameters,
			[changedParamInfo.name]: newValue,
		};
		setLocalParameters(newFormInputs);

		setButtonValues((prevButtonValues) => ({
			...(prevButtonValues || {}),
			[`param.${changedParamInfo.name}`]: newValue,
		}));

		const formInputsToSend: Record<string, string> = { ...newFormInputs };
		for (const p of parameters) {
			if (!(p.name in formInputsToSend)) {
				formInputsToSend[p.name] = p.value?.valid ? p.value.value : "";
			}
		}

		sendMessage(formInputsToSend);
	};

	useEffect(() => {
		if (!buttonValues && parameters.length === 0) {
			setButtonValues({ mode: "manual" });
		} else if (buttonValues && !buttonValues.mode && parameters.length > 0) {
			setButtonValues((prev) => ({ ...prev, mode: "manual" }));
		}
	}, [buttonValues, parameters]);

	if (!buttonValues || (!parameters && !error)) {
		return <Loader />;
	}

	return (
		<>
			<div className="flex items-start gap-12">
				<div className="flex flex-col gap-5 max-w-screen-md">
					{Boolean(error) && <ErrorAlert error={error} />}
					{diagnostics.length > 0 && <Diagnostics diagnostics={diagnostics} />}
					<div className="flex flex-col">
						<FormSection
							title="Creation mode"
							description="By changing the mode to automatic, when the user clicks the button, the workspace will be created automatically instead of showing a form to the user."
						>
							<RadioGroup
								defaultValue={buttonValues?.mode || "manual"}
								onChange={(_, v) => {
									setButtonValues((prevButtonValues) => ({
										...(prevButtonValues || {}),
										mode: v,
									}));
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
							<div className="flex flex-col gap-9">
								{parameters.map((parameter) => {
									const isDisabled = parameter.styling?.disabled;
									return (
										<DynamicParameter
											key={parameter.name}
											parameter={parameter}
											onChange={(value) => handleChange(parameter, value)}
											disabled={isDisabled}
											value={localParameters[parameter.name] || ""}
										/>
									);
								})}
							</div>
						)}
					</div>
				</div>

				<ButtonPreview template={template} buttonValues={buttonValues} />
			</div>
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
	const createWorkspaceParams = new URLSearchParams(buttonValues);
	const buttonUrl = `${createWorkspaceUrl}?${createWorkspaceParams.toString()}`;

	return `[![Open in Coder](${deploymentUrl}/open-in-coder.svg)](${buttonUrl})`;
}

interface ButtonPreviewProps {
	template: Template;
	buttonValues: ButtonValues | undefined;
}

const ButtonPreview: FC<ButtonPreviewProps> = ({ template, buttonValues }) => {
	const clipboard = useClipboard({
		textToCopy: getClipboardCopyContent(
			template.name,
			template.organization_name,
			buttonValues,
		),
	});

	return (
		<div
			className="sticky top-10 flex gap-16 h-80 p-14 flex-1 flex-col items-center justify-center
			 rounded-lg border border-border border-solid bg-surface-secondary"
		>
			<img src="/open-in-coder.svg" alt="Open in Coder button" />
			<Button
				variant="default"
				onClick={clipboard.copyToClipboard}
				disabled={clipboard.showCopiedSuccess}
			>
				{clipboard.showCopiedSuccess ? <CheckOutlined /> : <FileCopyOutlined />}{" "}
				Copy button code
			</Button>
		</div>
	);
};

export default TemplateEmbedPageExperimental;
