import CheckOutlined from "@mui/icons-material/CheckOutlined";
import FileCopyOutlined from "@mui/icons-material/FileCopyOutlined";
import Button from "@mui/material/Button";
import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import { API } from "api/api";
import { DetailedError } from "api/errors";
import type { 
  DynamicParametersRequest, 
  DynamicParametersResponse, 
  PreviewParameter, 
  Template 
} from "api/typesGenerated";
import { FormSection, VerticalForm } from "components/Form/Form";
import { Loader } from "components/Loader/Loader";
import { useClipboard } from "hooks/useClipboard";
import { DynamicParameter } from "modules/workspaces/DynamicParameter/DynamicParameter";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { type FC, useCallback, useEffect, useRef, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { getAutofillParameters } from "utils/richParameters";

type ButtonValues = Record<string, string>;

const TemplateEmbedPageExperimental: FC = () => {
  const { template } = useTemplateLayoutContext();
  const [searchParams] = useSearchParams();
  
  return (
    <>
      <Helmet>
        <title>{pageTitle(template.name)}</title>
      </Helmet>
      <TemplateEmbedPageView template={template} searchParams={searchParams} />
    </>
  );
};

interface TemplateEmbedPageViewProps {
  template: Template;
  searchParams: URLSearchParams;
}

const TemplateEmbedPageView: FC<TemplateEmbedPageViewProps> = ({ 
  template,
  searchParams
}) => {
  const [currentResponse, setCurrentResponse] = useState<DynamicParametersResponse | null>(null);
  const [wsResponseId, setWSResponseId] = useState<number>(-1);
  const ws = useRef<WebSocket | null>(null);
  const [wsError, setWsError] = useState<Error | null>(null);
  const [buttonValues, setButtonValues] = useState<ButtonValues | undefined>();
  
  // Get the current user
  const { data: me } = useQuery({
    queryKey: ["me"],
    queryFn: () => API.getAuthenticatedUser(),
  });

  // Check if workspace should be auto-created
  const isAutoMode = searchParams.get("mode") === "auto";

  // Parse autofill parameters from URL
  const autofillParameters = searchParams ? getAutofillParameters(searchParams) : [];

  const onMessage = useCallback((response: DynamicParametersResponse) => {
    setCurrentResponse((prev) => {
      if (prev?.id === response.id) {
        return prev;
      }
      return response;
    });
  }, []);

  // Initialize the WebSocket connection when component mounts
  useEffect(() => {
    if (!me?.id || !template.active_version_id) {
      return;
    }

    // If mode=auto and workspace will be auto-created, no need for WebSocket
    if (isAutoMode) {
      return;
    }

    const socket = API.templateVersionDynamicParameters(
      me.id,
      template.active_version_id,
      {
        onMessage,
        onError: (error) => {
          setWsError(error);
        },
        onClose: () => {
          // There is no reason for the websocket to close while a user is on the page
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
  }, [me?.id, template.active_version_id, onMessage, isAutoMode]);

  // Function to send messages to websocket
  const sendMessage = useCallback((formValues: Record<string, string>) => {
    setWSResponseId((prevId) => {
      const request: DynamicParametersRequest = {
        id: prevId + 1,
        inputs: formValues,
      };
      if (ws.current && ws.current.readyState === WebSocket.OPEN) {
        ws.current.send(JSON.stringify(request));
        return prevId + 1;
      }
      return prevId;
    });
  }, []);

  // Initialize button values when parameters are loaded
  useEffect(() => {
    if (currentResponse?.parameters && !buttonValues) {
      const mode = searchParams.get("mode") || "manual";
      const initValues: ButtonValues = {
        mode,
      };
      
      // Filter only parameters used for workspace creation
      const workspaceParams = currentResponse.parameters.filter(param => !param.ephemeral);
      
      // Apply autofill parameters from URL if available
      for (const parameter of workspaceParams) {
        const autofillParam = autofillParameters.find(p => p.name === parameter.name);
        
        if (autofillParam) {
          // Use the value from URL parameters
          initValues[`param.${parameter.name}`] = autofillParam.value;
        } else {
          // Use the default or current value from the parameter
          const paramValue = parameter.value.valid 
            ? parameter.value.value 
            : (parameter.default_value.valid ? parameter.default_value.value : "");
          
          initValues[`param.${parameter.name}`] = paramValue;
        }
      }
      
      setButtonValues(initValues);
      
      // Send initial message to get updated parameters based on autofill values
      if (workspaceParams.length > 0) {
        const paramInputs: Record<string, string> = {};
        
        for (const param of workspaceParams) {
          const autofillParam = autofillParameters.find(p => p.name === param.name);
          
          if (autofillParam) {
            paramInputs[param.name] = autofillParam.value;
          } else {
            paramInputs[param.name] = param.value.valid 
              ? param.value.value 
              : (param.default_value.valid ? param.default_value.value : "");
          }
        }
        
        sendMessage(paramInputs);
      }
    }
  }, [currentResponse, buttonValues, searchParams, autofillParameters, sendMessage]);

  // When no WebSocket connection is needed (auto mode), initialize buttonValues directly
  useEffect(() => {
    if (isAutoMode && !buttonValues && me) {
      const initValues: ButtonValues = {
        mode: "auto",
      };

      // Add autofill parameters to button values
      for (const param of autofillParameters) {
        initValues[`param.${param.name}`] = param.value;
      }

      setButtonValues(initValues);

      // No need to set currentResponse as we're not using the WebSocket in auto mode
    }
  }, [isAutoMode, buttonValues, me, autofillParameters]);

  const isLoading = (!buttonValues || (!currentResponse && !isAutoMode));

  return (
    <>
      {isLoading ? (
        <Loader />
      ) : (
        <div css={{ display: "flex", alignItems: "flex-start", gap: 48 }}>
          <div css={{ flex: 1, maxWidth: 400 }}>
            {wsError && (
              <div css={{ marginBottom: 16, color: "red" }}>
                <strong>Error: </strong> {wsError.message}
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
                    setButtonValues((buttonValues) => ({
                      ...buttonValues,
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

              {currentResponse?.parameters && (
                <ParametersList 
                  parameters={currentResponse.parameters}
                  buttonValues={buttonValues || {}}
                  setButtonValues={setButtonValues}
                  sendMessage={sendMessage}
                  autofillParameters={autofillParameters}
                />
              )}
            </VerticalForm>
          </div>

          <ButtonPreview 
            template={template}
            buttonValues={buttonValues}
          />
        </div>
      )}
    </>
  );
};

interface ParametersListProps {
  parameters: PreviewParameter[];
  buttonValues: ButtonValues;
  setButtonValues: (values: ButtonValues | ((prev: ButtonValues) => ButtonValues)) => void;
  sendMessage: (values: Record<string, string>) => void;
  autofillParameters: AutofillBuildParameter[];
}

const ParametersList: FC<ParametersListProps> = ({
  parameters,
  buttonValues,
  setButtonValues,
  sendMessage,
  autofillParameters,
}) => {
  // Filter parameters to only include those used for workspace creation
  const workspaceParameters = parameters.filter(param => !param.ephemeral);
  
  if (workspaceParameters.length === 0) {
    return null;
  }

  // Handle parameter change
  const handleParameterChange = (paramName: string, value: string) => {
    // Update button values
    setButtonValues((prev) => ({
      ...prev,
      [`param.${paramName}`]: value,
    }));
    
    // Send updated parameters to the server
    const paramValues: Record<string, string> = {};
    for (const param of workspaceParameters) {
      if (param.name === paramName) {
        paramValues[param.name] = value;
      } else {
        const paramKey = `param.${param.name}`;
        paramValues[param.name] = buttonValues[paramKey] || "";
      }
    }
    sendMessage(paramValues);
  };

  return (
    <div css={{ display: "flex", flexDirection: "column", gap: 36 }}>
      {workspaceParameters.map((parameter) => {
        const autofillParam = autofillParameters.find(p => p.name === parameter.name);
        const isAutofilled = !!autofillParam;
        
        return (
          <DynamicParameter
            key={parameter.name}
            parameter={parameter}
            onChange={(value) => handleParameterChange(parameter.name, value)}
            disabled={isAutofilled}
          />
        );
      })}
    </div>
  );
};

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
      css={(theme) => ({
        // 80px for padding, 36px is for the status bar. We want to use `vh`
        // so that it will be relative to the screen and not the parent layout.
        height: "calc(100vh - (80px + 36px))",
        top: 40,
        position: "sticky",
        display: "flex",
        padding: 64,
        flex: 1,
        alignItems: "center",
        justifyContent: "center",
        borderRadius: 8,
        backgroundColor: theme.palette.background.paper,
        border: `1px solid ${theme.palette.divider}`,
      })}
    >
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
          startIcon={
            clipboard.showCopiedSuccess ? (
              <CheckOutlined />
            ) : (
              <FileCopyOutlined />
            )
          }
          variant="contained"
          onClick={clipboard.copyToClipboard}
          disabled={clipboard.showCopiedSuccess}
        >
          Copy button code
        </Button>
      </div>
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

// Function is now imported from utils/richParameters.ts

export default TemplateEmbedPageExperimental;