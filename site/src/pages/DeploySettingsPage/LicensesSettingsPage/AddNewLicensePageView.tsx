import Button from "@material-ui/core/Button"
import TextField from "@material-ui/core/TextField"
import { makeStyles } from "@material-ui/core/styles"
import { useMutation } from "@tanstack/react-query"
import { createLicense } from "api/api"
import { Fieldset } from "components/DeploySettingsLayout/Fieldset"
import { Header } from "components/DeploySettingsLayout/Header"
import { DividerWithText } from "components/DividerWithText/DividerWithText"
import { FileUpload } from "components/FileUpload/FileUpload"
import { Form, FormFields, FormSection } from "components/Form/Form"
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { Link as RouterLink, useNavigate } from "react-router-dom"

const AddNewLicense: FC = () => {
  const styles = useStyles()
  const navigate = useNavigate()

  const {
    mutate: saveLicenseKeyApi,
    isLoading: isCreating,
    isError: creationFailed,
  } = useMutation(createLicense)

  function handleFileUploaded(files: File[]) {
    const fileReader = new FileReader()
    fileReader.onload = () => {
      const licenseKey = fileReader.result as string

      saveLicenseKey(licenseKey)

      fileReader.onerror = () => {
        displayError("Failed to read file")
      }
    }

    fileReader.readAsText(files[0])
  }

  function saveLicenseKey(licenseKey: string) {
    saveLicenseKeyApi(
      { license: licenseKey },
      {
        onSuccess: () => {
          displaySuccess("You have successfully added a license")
          navigate("/settings/deployment/licenses?success=true")
        },
        onError: () => displayError("Failed to save license key"),
      },
    )
  }

  const isUploading = false

  function onUpload(file: File) {
    handleFileUploaded([file])
  }

  return (
    <>
      <Stack
        alignItems="baseline"
        direction="row"
        justifyContent="space-between"
      >
        <Header
          title="Add your license"
          description="Enterprise licenses unlock more features on your deployment."
        />
        <Button
          component={RouterLink}
          to="/settings/deployment/licenses"
          variant="outlined"
        >
          Back to licenses
        </Button>
      </Stack>

      <Form direction="horizontal">
        <FormSection
          title="Upload your license"
          description="Upload a text file containing your license key"
        >
          <FormFields>
            <FileUpload
              isUploading={isUploading}
              onUpload={onUpload}
              file={undefined}
              removeLabel="Remove File"
              title="Upload a license"
            />
          </FormFields>
        </FormSection>
      </Form>

      <Stack className={styles.main}>
        <DividerWithText>or</DividerWithText>

        <Fieldset
          title="Paste your license key"
          validation={creationFailed ? "License key is invalid" : undefined}
          onSubmit={(e) => {
            e.preventDefault()

            const form = e.target
            const formData = new FormData(form as HTMLFormElement)

            const licenseKey = formData.get("licenseKey")

            saveLicenseKey(licenseKey?.toString() || "")
          }}
          button={
            <Button type="submit" disabled={isCreating}>
              Add license
            </Button>
          }
        >
          <TextField
            name="licenseKey"
            placeholder="Paste your license key here"
            multiline
            rows={4}
            fullWidth
          />
        </Fieldset>
      </Stack>
    </>
  )
}

export default AddNewLicense

const useStyles = makeStyles((theme) => ({
  main: {
    paddingTop: theme.spacing(5),
  },
  ctaButton: {
    backgroundImage: `linear-gradient(90deg, ${theme.palette.secondary.main} 0%, ${theme.palette.secondary.dark} 100%)`,
    width: theme.spacing(30),
    marginBottom: theme.spacing(4),
  },
  formSectionRoot: {
    alignItems: "center",
  },
  description: {
    color: theme.palette.text.secondary,
    lineHeight: "160%",
  },
  title: {
    ...theme.typography.h5,
    fontWeight: 600,
    marging: theme.spacing(1),
  },
}))
