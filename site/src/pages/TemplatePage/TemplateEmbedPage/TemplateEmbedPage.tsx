import CheckOutlined from "@mui/icons-material/CheckOutlined"
import FileCopyOutlined from "@mui/icons-material/FileCopyOutlined"
import Box from "@mui/material/Box"
import Button from "@mui/material/Button"
import { useQuery } from "@tanstack/react-query"
import { getTemplateVersionRichParameters } from "api/api"
import { VerticalForm } from "components/Form/Form"
import { Loader } from "components/Loader/Loader"
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import {
  ImmutableTemplateParametersSection,
  MultableTemplateParametersSection,
  TemplateParametersSectionProps,
} from "components/TemplateParameters/TemplateParameters"
import { useClipboard } from "hooks/useClipboard"
import { useState } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import {
  selectInitialRichParametersValues,
  workspaceBuildParameterValue,
} from "utils/richParameters"

type ButtonValues = Record<string, string>

const TemplateEmbedPage = () => {
  const { template } = useTemplateLayoutContext()
  const { data: templateParameters, isLoading } = useQuery({
    queryKey: ["template", template.id, "embed"],
    queryFn: () => getTemplateVersionRichParameters(template.active_version_id),
  })
  const [buttonValues, setButtonValues] = useState<ButtonValues>({})
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
      {isLoading ? (
        <Loader />
      ) : (
        <Box display="flex" alignItems="flex-start" gap={6}>
          {templateParameters && (
            <Box flex={1} maxWidth={400}>
              <VerticalForm>
                <MultableTemplateParametersSection
                  templateParameters={templateParameters}
                  getInputProps={getInputProps}
                />
                <ImmutableTemplateParametersSection
                  templateParameters={templateParameters}
                  getInputProps={getInputProps}
                />
              </VerticalForm>
            </Box>
          )}
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
