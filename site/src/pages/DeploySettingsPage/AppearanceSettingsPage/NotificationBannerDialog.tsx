import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import DialogActions from "@mui/material/DialogActions";
import TextField from "@mui/material/TextField";
import { useFormik } from "formik";
import type { FC } from "react";
import { BlockPicker } from "react-color";
import type { BannerConfig } from "api/typesGenerated";
import { Dialog, DialogActionButtons } from "components/Dialogs/Dialog";
import { Stack } from "components/Stack/Stack";
import { NotificationBannerView } from "modules/dashboard/NotificationBanners/NotificationBannerView";
import { getFormHelpers } from "utils/formUtils";

interface NotificationBannerDialogProps {
  banner: BannerConfig;
  onCancel: () => void;
  onUpdate: (banner: Partial<BannerConfig>) => Promise<void>;
}

export const NotificationBannerDialog: FC<NotificationBannerDialogProps> = ({
  banner,
  onCancel,
  onUpdate,
}) => {
  const theme = useTheme();

  const bannerForm = useFormik<{
    message: string;
    background_color: string;
  }>({
    initialValues: {
      message: banner.message ?? "",
      background_color: banner.background_color ?? "#004852",
    },
    onSubmit: (banner) => onUpdate(banner),
  });
  const bannerFieldHelpers = getFormHelpers(bannerForm);

  return (
    <Dialog css={styles.dialogWrapper} open onClose={onCancel}>
      {/* Banner preview */}
      <div css={{ position: "fixed", top: 0, left: 0, right: 0 }}>
        <NotificationBannerView
          message={bannerForm.values.message}
          backgroundColor={bannerForm.values.background_color}
        />
      </div>

      <div css={styles.dialogContent}>
        <h3 css={styles.dialogTitle}>Notification banner</h3>
        <Stack>
          <div>
            <h4 css={styles.settingName}>Message</h4>
            <TextField
              {...bannerFieldHelpers("message", {
                helperText: "Markdown bold, italics, and links are supported.",
              })}
              fullWidth
              inputProps={{
                "aria-label": "Message",
                placeholder: "Enter a message for the banner",
              }}
            />
          </div>
          <div>
            <h4 css={styles.settingName}>Background color</h4>
            <BlockPicker
              color={bannerForm.values.background_color}
              onChange={async (color) => {
                await bannerForm.setFieldValue("background_color", color.hex);
              }}
              triangle="hide"
              colors={["#004852", "#D65D0F", "#4CD473", "#D94A5D", "#5A00CF"]}
              styles={{
                default: {
                  input: {
                    color: "white",
                    backgroundColor: theme.palette.background.default,
                  },
                  body: {
                    backgroundColor: "black",
                    color: "white",
                  },
                  card: {
                    backgroundColor: "black",
                  },
                },
              }}
            />
          </div>
        </Stack>
      </div>

      <DialogActions>
        <DialogActionButtons
          cancelText="Cancel"
          confirmLoading={bannerForm.isSubmitting}
          confirmText="Update"
          disabled={bannerForm.isSubmitting}
          onCancel={onCancel}
          onConfirm={bannerForm.handleSubmit}
        />
      </DialogActions>
    </Dialog>
  );
};

const styles = {
  dialogWrapper: (theme) => ({
    "& .MuiPaper-root": {
      background: theme.palette.background.paper,
      border: `1px solid ${theme.palette.divider}`,
      width: "100%",
      maxWidth: 500,
    },
    "& .MuiDialogActions-spacing": {
      padding: "0 40px 40px",
    },
  }),
  dialogContent: (theme) => ({
    color: theme.palette.text.secondary,
    padding: "40px 40px 20px",
  }),
  dialogTitle: (theme) => ({
    margin: 0,
    marginBottom: 16,
    color: theme.palette.text.primary,
    fontWeight: 400,
    fontSize: 20,
  }),
  settingName: (theme) => ({
    marginTop: 0,
    marginBottom: 8,
    color: theme.palette.text.primary,
    fontSize: 16,
    lineHeight: "150%",
    fontWeight: 600,
  }),
} satisfies Record<string, Interpolation<Theme>>;
