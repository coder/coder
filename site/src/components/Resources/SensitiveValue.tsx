import IconButton from "@material-ui/core/IconButton"
import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import VisibilityOffOutlined from "@material-ui/icons/VisibilityOffOutlined"
import VisibilityOutlined from "@material-ui/icons/VisibilityOutlined"
import { useState } from "react"

const Language = {
  showLabel: "Show value",
  hideLabel: "Hide value",
}

export const SensitiveValue: React.FC<{ value: string }> = ({ value }) => {
  const [shouldDisplay, setShouldDisplay] = useState(false)
  const styles = useStyles()
  const displayValue = shouldDisplay ? value : "••••••••"
  const buttonLabel = shouldDisplay ? Language.hideLabel : Language.showLabel
  const icon = shouldDisplay ? (
    <VisibilityOffOutlined />
  ) : (
    <VisibilityOutlined />
  )

  return (
    <div className={styles.sensitiveValue}>
      {displayValue}
      <Tooltip title={buttonLabel}>
        <IconButton
          className={styles.button}
          onClick={() => {
            setShouldDisplay((value) => !value)
          }}
          size="small"
          aria-label={buttonLabel}
        >
          {icon}
        </IconButton>
      </Tooltip>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  sensitiveValue: {
    display: "flex",
    alignItems: "center",
  },

  button: {
    marginLeft: theme.spacing(0.5),
    color: "inherit",

    "& .MuiSvgIcon-root": {
      width: 16,
      height: 16,
    },
  },
}))
