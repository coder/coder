import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { FC } from "react"
import { CoderIcon } from "../Icons/CoderIcon"

export const Welcome: FC = () => {
  const styles = useStyles()

  return (
    <div>
      <div className={styles.logoBox}>
        <CoderIcon className={styles.logo} />
      </div>
      <Typography className={styles.title} variant="h1">
        Welcome to <strong>Coder</strong>
      </Typography>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  logoBox: {
    display: "flex",
    justifyContent: "center",
  },
  logo: {
    width: 80,
    height: 56,
    color: theme.palette.text.primary,
  },
  title: {
    fontSize: 24,
    letterSpacing: -0.3,
    marginBottom: theme.spacing(3),
    marginTop: theme.spacing(6),
    textAlign: "center",
  },
}))
