import CheckOutlined from "@mui/icons-material/CheckOutlined"
import FileCopyOutlined from "@mui/icons-material/FileCopyOutlined"
import Box from "@mui/material/Box"
import Button from "@mui/material/Button"
import FormControlLabel from "@mui/material/FormControlLabel"
import Radio from "@mui/material/Radio"
import RadioGroup from "@mui/material/RadioGroup"
import { useQuery } from "@tanstack/react-query"
import { getTemplateVersionRichParameters } from "api/api"
import { Template, TemplateVersionParameter } from "api/typesGenerated"
import { FormSection, VerticalForm } from "components/Form/Form"
import { Loader } from "components/Loader/Loader"
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import {
  ImmutableTemplateParametersSection,
  MutableTemplateParametersSection,
  TemplateParametersSectionProps,
} from "components/TemplateParameters/TemplateParameters"
import { useClipboard } from "hooks/useClipboard"
import { FC, useState } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import {
  selectInitialRichParametersValues,
  workspaceBuildParameterValue,
} from "utils/richParameters"
import { paramsUsedToCreateWorkspace } from "utils/workspace"

type ButtonValues = Record<string, string>

const TemplateEmbedPage = () => {
  const { template } = useTemplateLayoutContext()
  const { data: templateParameters } = useQuery({
    queryKey: ["template", template.id, "embed"],
    queryFn: () => getTemplateVersionRichParameters(template.active_version_id),
  })

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Embed`)}</title>
      </Helmet>
      <TemplateEmbedPageView
        template={template}
        templateParameters={templateParameters?.filter(
          paramsUsedToCreateWorkspace,
        )}
      />
    </>
  )
}

export const TemplateEmbedPageView: FC<{
  template: Template
  templateParameters?: TemplateVersionParameter[]
}> = ({ template, templateParameters }) => {
  const [buttonValues, setButtonValues] = useState<ButtonValues>({
    mode: "manual",
  })
  const initialRichParametersValues = templateParameters
    ? selectInitialRichParametersValues(templateParameters)
    : undefined
  const deploymentUrl = `${window.location.protocol}//${window.location.host}`
  const createWorkspaceUrl = `${deploymentUrl}/templates/${template.name}/workspace`
  const createWorkspaceParams = new URLSearchParams(buttonValues)
  const buttonUrl = `${createWorkspaceUrl}?${createWorkspaceParams.toString()}`
  const buttonMkdCode = `[![Open in Coder](${deploymentUrl}/open-in-coder.svg)](${buttonUrl})`
  const clipboard = useClipboard(buttonMkdCode)

  const getInputProps: TemplateParametersSectionProps["getInputProps"] = (
    parameter,
  ) => {
    if (!initialRichParametersValues) {
      throw new Error("initialRichParametersValues is undefined")
    }
    return {
      id: parameter.name,
      initialValue: workspaceBuildParameterValue(
        initialRichParametersValues,
        parameter,
      ),
      onChange: (value) => {
        setButtonValues((buttonValues) => ({
          ...buttonValues,
          [`param.${parameter.name}`]: value,
        }))
      },
    }
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Embed`)}</title>
      </Helmet>
      {!templateParameters ? (
        <Loader />
      ) : (
        <Box display="flex" alignItems="flex-start" gap={6}>
          <Box flex={1} maxWidth={400}>
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
                    }))
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

              {templateParameters.length > 0 && (
                <>
                  <MutableTemplateParametersSection
                    templateParameters={templateParameters}
                    getInputProps={getInputProps}
                  />
                  <ImmutableTemplateParametersSection
                    templateParameters={templateParameters}
                    getInputProps={getInputProps}
                  />
                </>
              )}
            </VerticalForm>
          </Box>
          <Box
            display="flex"
            height={{
              // 80px is the vertical padding of the content area
              // 36px is from the status bar from the bottom
              md: "calc(100vh - (80px + 36px))",
              top: 40,
              position: "sticky",
            }}
            p={8}
            flex={1}
            alignItems="center"
            justifyContent="center"
            borderRadius={1}
            bgcolor="background.paper"
            border={(theme) => `1px solid ${theme.palette.divider}`}
          >
            <img src="/open-in-coder.svg" alt="Open in Coder button" />
            <Box
              p={2}
              py={6}
              display="flex"
              justifyContent="center"
              position="absolute"
              bottom={0}
              left={0}
              width="100%"
            >
              <Button
                sx={{ borderRadius: 999 }}
                startIcon={
                  clipboard.isCopied ? <CheckOutlined /> : <FileCopyOutlined />
                }
                variant="contained"
                onClick={clipboard.copy}
                disabled={clipboard.isCopied}
              >
                Copy button code
              </Button>
            </Box>
          </Box>
        </Box>
      )}
    </>
  )
}

export default TemplateEmbedPage
