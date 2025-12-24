import { API } from "api/api";
import { DetailedError } from "api/errors";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
	FriendlyDiagnostic,
	PreviewParameter,
	Template,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { Label } from "components/Label/Label";
import { RadioGroup, RadioGroupItem } from "components/RadioGroup/RadioGroup";
import { Separator } from "components/Separator/Separator";
import { Skeleton } from "components/Skeleton/Skeleton";
import { useAuthenticated } from "hooks";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useClipboard } from "hooks/useClipboard";
import { CheckIcon, CopyIcon } from "lucide-react";
import {
	Diagnostics,
	DynamicParameter,
} from "modules/workspaces/DynamicParameter/DynamicParameter";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { type FC, useEffect, useMemo, useRef, useState } from "react";
import { pageTitle } from "utils/page";

type ButtonValues = Record<string, string>;

const TemplateEmbedPageExperimental: FC = () => {
	const { template } = useTemplateLayoutContext();
	const { user: me } = useAuthenticated();
	const [latestResponse, setLatestResponse] =
		useState<DynamicParametersResponse | null>(null);
	const wsResponseId = useRef<number>(-1);
	const ws = useRef<WebSocket | null>(null);
	const [wsError, setWsError] = useState<Error | null>(null);

	const sendMessage = useEffectEvent(
		(formValues: Record<string, string>, _ownerId?: string) => {
			const request: DynamicParametersRequest = {
				id: wsResponseId.current + 1,
				owner_id: me.id,
				inputs: formValues,
			};
			if (ws.current && ws.current.readyState === WebSocket.OPEN) {
				ws.current.send(JSON.stringify(request));
				wsResponseId.current = wsResponseId.current + 1;
			}
		},
	);

	const onMessage = useEffectEvent((response: DynamicParametersResponse) => {
		if (latestResponse && latestResponse?.id >= response.id) {
			return;
		}

		setLatestResponse(response);
	});

	useEffect(() => {
		if (!template.active_version_id || !me) {
			return;
		}

		const socket = API.templateVersionDynamicParameters(
			template.active_version_id,
			me.id,
			{
				onMessage,
				onError: (error) => {
					if (ws.current === socket) {
						setWsError(error);
					}
				},
				onClose: () => {
					if (ws.current === socket) {
						setWsError(
							new DetailedError(
								"Websocket connection for dynamic parameters unexpectedly closed.",
								"Refresh the page to reset the form.",
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
	}, [template.active_version_id, onMessage, me]);

	const sortedParams = useMemo(() => {
		if (!latestResponse?.parameters) {
			return [];
		}
		return [...latestResponse.parameters].sort((a, b) => a.order - b.order);
	}, [latestResponse?.parameters]);

	const isLoading =
		ws.current?.readyState === WebSocket.CONNECTING || !latestResponse;

	return (
		<>
			<title>{pageTitle(template.name)}</title>

			<TemplateEmbedPageView
				template={template}
				parameters={sortedParams}
				diagnostics={latestResponse?.diagnostics ?? []}
				error={wsError}
				sendMessage={sendMessage}
				isLoading={isLoading}
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
	isLoading: boolean;
}

const TemplateEmbedPageView: FC<TemplateEmbedPageViewProps> = ({
	template,
	parameters,
	diagnostics,
	error,
	sendMessage,
	isLoading,
}) => {
	const [formState, setFormState] = useState<{
		mode: "manual" | "auto";
		paramValues: Record<string, string>;
	}>({
		mode: "manual",
		paramValues: {},
	});

	useEffect(() => {
		if (parameters) {
			const serverParamValues: Record<string, string> = {};
			for (const p of parameters) {
				const initialVal = p.value?.valid ? p.value.value : "";
				serverParamValues[p.name] = initialVal;
			}
			setFormState((prev) => ({ ...prev, paramValues: serverParamValues }));
		}
	}, [parameters]);

	const buttonValues = useMemo(() => {
		const values: ButtonValues = { mode: formState.mode };
		for (const [key, value] of Object.entries(formState.paramValues)) {
			values[`param.${key}`] = value;
		}
		return values;
	}, [formState]);

	const handleChange = (
		changedParamInfo: PreviewParameter,
		newValue: string,
	) => {
		const newParamValues = {
			...formState.paramValues,
			[changedParamInfo.name]: newValue,
		};
		setFormState((prev) => ({ ...prev, paramValues: newParamValues }));

		const formInputsToSend: Record<string, string> = { ...newParamValues };
		for (const p of parameters) {
			if (!(p.name in formInputsToSend)) {
				formInputsToSend[p.name] = p.value?.valid ? p.value.value : "";
			}
		}

		sendMessage(formInputsToSend);
	};

	return (
		<div className="flex items-start gap-12">
			<div className="w-full flex flex-col gap-5 max-w-screen-md">
				{isLoading ? (
					<div className="flex flex-col gap-9">
						<div className="flex flex-col gap-2">
							<Skeleton className="h-5 w-1/3" />
							<Skeleton className="h-9 w-full" />
						</div>
						<div className="flex flex-col gap-2">
							<Skeleton className="h-5 w-1/3" />
							<Skeleton className="h-9 w-full" />
						</div>
						<div className="flex flex-col gap-2">
							<Skeleton className="h-5 w-1/3" />
							<Skeleton className="h-9 w-full" />
						</div>
					</div>
				) : (
					<>
						{Boolean(error) && <ErrorAlert error={error} />}
						{diagnostics.length > 0 && (
							<Diagnostics diagnostics={diagnostics} />
						)}
						<div className="flex flex-col gap-9">
							<section className="flex flex-col gap-2">
								<div>
									<h2 className="text-lg font-bold m-0">Creation mode</h2>
									<p className="text-sm text-content-secondary m-0">
										When set to automatic mode, clicking the button will create
										the workspace automatically without displaying a form to the
										user.
									</p>
								</div>
								<RadioGroup
									value={formState.mode}
									onValueChange={(v) => {
										setFormState((prev) => ({
											...prev,
											mode: v as "manual" | "auto",
										}));
									}}
								>
									<div className="flex items-center gap-3">
										<RadioGroupItem value="manual" id="manual" />
										<Label htmlFor="manual" className="cursor-pointer">
											Manual
										</Label>
									</div>
									<div className="flex items-center gap-3">
										<RadioGroupItem value="auto" id="automatic" />
										<Label htmlFor="automatic" className="cursor-pointer">
											Automatic
										</Label>
									</div>
								</RadioGroup>
							</section>

							<Separator />

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
												value={formState.paramValues[parameter.name] || ""}
											/>
										);
									})}
								</div>
							)}
						</div>
					</>
				)}
			</div>

			<ButtonPreview template={template} buttonValues={buttonValues} />
		</div>
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
	const clipboard = useClipboard();
	return (
		<div
			className="sticky top-10 flex gap-16 h-96 flex-1 flex-col items-center justify-center
			 rounded-lg border border-border border-solid bg-surface-secondary"
		>
			<img src="/open-in-coder.svg" alt="Open in Coder button" />
			<Button
				variant="default"
				disabled={clipboard.showCopiedSuccess}
				onClick={() => {
					const textToCopy = getClipboardCopyContent(
						template.name,
						template.organization_name,
						buttonValues,
					);
					clipboard.copyToClipboard(textToCopy);
				}}
			>
				{clipboard.showCopiedSuccess ? <CheckIcon /> : <CopyIcon />} Copy button
				code
			</Button>
		</div>
	);
};

export default TemplateEmbedPageExperimental;
