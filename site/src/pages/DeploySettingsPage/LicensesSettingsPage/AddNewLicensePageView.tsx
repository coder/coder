import Button from "@material-ui/core/Button"
import TextField from "@material-ui/core/TextField"
import { makeStyles } from "@material-ui/core/styles"
import { ApiErrorResponse } from "api/errors"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Fieldset } from "components/DeploySettingsLayout/Fieldset"
import { Header } from "components/DeploySettingsLayout/Header"
import { FileUpload } from "components/FileUpload/FileUpload"
import { displayError } from "components/GlobalSnackbar/utils"
import { Stack } from "components/Stack/Stack"
import { DividerWithText } from "pages/DeploySettingsPage/LicensesSettingsPage/DividerWithText"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"

type AddNewLicenseProps = {
  onSaveLicenseKey: (license: string) => void
  isSavingLicense: boolean
  savingLicenseError?: ApiErrorResponse
}

export const AddNewLicensePageView: FC<AddNewLicenseProps> = ({
  onSaveLicenseKey,
  isSavingLicense,
  savingLicenseError,
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

      {savingLicenseError && (
        <AlertBanner severity="error" error={savingLicenseError}></AlertBanner>
      )}

      <FileUpload
        isUploading={isUploading}
        onUpload={onUpload}
        removeLabel="Remove File"
        title="Upload your license"
        description="Upload a text file containing your license key"
      />

      <Stack className={styles.main}>
        <DividerWithText>or</DividerWithText>

        <Fieldset
          title="Paste your license key"
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
