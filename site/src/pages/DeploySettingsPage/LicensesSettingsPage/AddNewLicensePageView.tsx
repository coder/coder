
import { Stack } from "components/Stack/Stack"
import { FC, PropsWithChildren, useState } from "react"
import { Header } from "components/DeploySettingsLayout/Header"
import { FormSection, HorizontalForm } from "components/Form/Form"
import Button from "@material-ui/core/Button"
import { DropzoneDialog } from "material-ui-dropzone"
import { Divider, Input, TextareaAutosize, makeStyles } from "@material-ui/core"
import { Fieldset } from "components/DeploySettingsLayout/Fieldset"

const AddNewLicense: FC = () => {

  return (
    <>
      <Stack alignItems="baseline" direction="row" justifyContent="space-between">
        <Header
          title="Add your license"
          description="Add a license to your account to unlock more features."
        />
      </Stack>


      <HorizontalForm
        onSubmit={(data: unknown) => {
          console.log(data)
        }}
      >
        <FormSection
          title="Upload license file"
          description="please upload the license file you received when you purchased your license."
        >


        <DropzoneDialogExample />

        </FormSection>

        <DividerWithText>or</DividerWithText>

        <Fieldset
          title="Paste your license key"
          onSubmit={(data: unknown) => { console.log(data) }}
        >

          </Fieldset>
      </HorizontalForm>
    </>
  )
}

export default AddNewLicense

const useStyles = makeStyles(theme => ({
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
    fontSize:  theme.typography.h5.fontSize,
    color:  theme.palette.text.secondary
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

const DropzoneDialogExample = (props) => {
  const [open, setOpen] = useState(false);
  const [files, setFiles] = useState([]);

  function handleClose() {
    setOpen(false);
  }

  function handleSave(files) {
    setFiles(files);
    setOpen(false);
  }

  function handleOpen() {
    setOpen(true)
  }

  return (
    <div>
      <Button onClick={handleOpen}>
        Add Image
      </Button>
      <DropzoneDialog
        open={open}
        onSave={handleSave}
        acceptedFiles={['image/jpeg', 'image/png', 'image/bmp']}
        showPreviews={true}
        maxFileSize={5000000}
        onClose={handleClose}
      />
    </div>
  );
}
