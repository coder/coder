
import { makeStyles, useTheme } from "@material-ui/core/styles"
import Button from "@material-ui/core/Button"
import { Fieldset } from "components/DeploySettingsLayout/Fieldset"
import { Header } from "components/DeploySettingsLayout/Header"
import { FormFields, FormSection } from "components/Form/Form"
import { Stack } from "components/Stack/Stack"
import { DropzoneDialog } from "material-ui-dropzone"
import { FC, PropsWithChildren, useState } from "react"
import Confetti from 'react-confetti'
import { NavLink, Link as RouterLink } from "react-router-dom"
import { useToggle } from 'react-use'
import useWindowSize from 'react-use/lib/useWindowSize'
import TextField from "@material-ui/core/TextField"
import PlusOneOutlined from "@material-ui/icons/PlusOneOutlined"
import { CloudUploadOutlined } from "@material-ui/icons"

const AddNewLicense: FC = () => {
  const styles = useStyles()
  const { width, height } = useWindowSize()
  const [confettiOn, toggleConfettiOn] = useToggle(false)
  const [isDialogOpen, toggleDialogOpen] = useToggle(false)
  const [files, setFiles] = useState<File[]>([]);
  const theme = useTheme()

  function handleSave(files: File[]) {
    setFiles(files);
    console.log(files)
    toggleDialogOpen()
    toggleConfettiOn()
    setTimeout(() => {
      toggleConfettiOn(false)
    }, 2000)
  }

  return (
    <>
      <Confetti
        width={width}
        height={height}
        run={confettiOn}
        colors={[theme.palette.primary.main, theme.palette.secondary.main]}
      />
      <Stack alignItems="baseline" direction="row" justifyContent="space-between">
        <Header
          title="Add your license"
          description="Add a license to your account to unlock more features."
        />
        <Button
          component={RouterLink}
          to="/settings/deployment/licenses"
          variant="outlined"
        >
          Back to licenses
        </Button>
      </Stack>

      <Stack
        spacing={4}
      >
        <FormSection
          title="Upload license file"
          description="please upload the license file you received when you purchased your license."
          classes={{
            root: styles.formSectionRoot
          }}
        >


          <Stack style={{
            height: "100%",
          }}>
            <div>
              <Button
              startIcon={<CloudUploadOutlined />}
              size="large"
              variant="contained"
                onClick={() => toggleDialogOpen()}>
                Upload license file
              </Button>
              <DropzoneDialog
                open={isDialogOpen}
                onSave={handleSave}
                // acceptedFiles={['image/jpeg', 'image/png', 'image/bmp']}
                showPreviews={false}
                maxFileSize={1000000}
                onClose={() => toggleDialogOpen(false)}
              />
            </div>
          </Stack>

        </FormSection>
        <FormFields>

          <DividerWithText>or</DividerWithText>
        </FormFields>

        <Fieldset
          title="Paste your license key"
          onSubmit={(data: unknown) => {
            console.log(data)
          }}
        >
          <TextField placeholder="Paste your license key here" multiline rows={4} fullWidth />

        </Fieldset>
      </Stack>
    </>
  )
}

export default AddNewLicense


const useStyles = makeStyles(theme => ({
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
    marging: theme.spacing(1)
  },
  container: {
    display: "flex",
    alignItems: "center"
  },
  border: {
    borderBottom: `2px solid ${theme.palette.divider}`,
    width: "100%"
  },
  content: {
    paddingTop: theme.spacing(0.5),
    paddingBottom: theme.spacing(0.5),
    paddingRight: theme.spacing(2),
    paddingLeft: theme.spacing(2),
    fontWeight: 500,
    fontSize: theme.typography.h5.fontSize,
    color: theme.palette.text.secondary
  }
}));

const DividerWithText: FC<PropsWithChildren> = ({ children }) => {
  const classes = useStyles();
  return (
    <div className={classes.container}>
      <div className={classes.border} />
      <span className={classes.content}>{children}</span>
      <div className={classes.border} />
    </div>
  );
};
