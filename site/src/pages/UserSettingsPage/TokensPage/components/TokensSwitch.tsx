import { FC } from "react"
import Switch from "@material-ui/core/Switch"
import FormGroup from "@material-ui/core/FormGroup"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { useTranslation } from "react-i18next"

export const TokensSwitch: FC<{
  hasReadAll: boolean
  viewAllTokens: boolean
  setViewAllTokens: (arg: boolean) => void
}> = ({ hasReadAll, viewAllTokens, setViewAllTokens }) => {
  const styles = useStyles()
  const { t } = useTranslation("tokensPage")

  return (
    <FormGroup row className={styles.formRow}>
      {hasReadAll && (
        <FormControlLabel
          control={
            <Switch
              className={styles.selectAllSwitch}
              checked={viewAllTokens}
              onChange={() => setViewAllTokens(!viewAllTokens)}
              name="viewAllTokens"
              color="primary"
            />
          }
          label={t("toggleLabel")}
        />
      )}
    </FormGroup>
  )
}

const useStyles = makeStyles(() => ({
  formRow: {
    justifyContent: "end",
    marginBottom: "10px",
  },
  selectAllSwitch: {
    // decrease the hover state on the switch
    // so that it isn't hidden behind the container
    "& .MuiIconButton-root": {
      padding: "8px",
    },
  },
}))
