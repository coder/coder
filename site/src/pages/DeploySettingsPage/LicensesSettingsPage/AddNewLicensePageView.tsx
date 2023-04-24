import Button from "@material-ui/core/Button"
import TextField from "@material-ui/core/TextField"
import { makeStyles } from "@material-ui/core/styles"
import { Fieldset } from "components/DeploySettingsLayout/Fieldset"
import { Header } from "components/DeploySettingsLayout/Header"
import { DividerWithText } from "components/DividerWithText/DividerWithText"
import { FileUpload } from "components/FileUpload/FileUpload"
import { Form, FormFields, FormSection } from "components/Form/Form"
import { displayError } from "components/GlobalSnackbar/utils"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"

type AddNewLicenseProps = {
  onSaveLicenseKey: (license: string) => void
  isSavingLicense: boolean
  didSavingFailed: boolean
}

export const AddNewLicensePageView: FC<AddNewLicenseProps> = ({
  onSaveLicenseKey,
  isSavingLicense,
  didSavingFailed,
}) => {
  const styles = useStyles()

  function handleFileUploaded(files: File[]) {
    const fileReader = new FileReader()
    fileReader.onload = () => {
      const licenseKey = fileReader.result as string

      onSaveLicenseKey(licenseKey)

      fileReader.onerror = () => {
        displayError("Failed to read file")
      }
    }

    fileReader.readAsText(files[0])
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
          validation={didSavingFailed ? "License key is invalid" : undefined}
          onSubmit={(e) => {
            e.preventDefault()

            const form = e.target
            const formData = new FormData(form as HTMLFormElement)

            const licenseKey = formData.get("licenseKey")

            onSaveLicenseKey(licenseKey?.toString() || "")
          }}
          button={
            <Button type="submit" disabled={isSavingLicense}>
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

const useStyles = makeStyles((theme) => ({
  main: {
    paddingTop: theme.spacing(5),
  },
}))
