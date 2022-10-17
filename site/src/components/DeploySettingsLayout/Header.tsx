import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import LaunchOutlined from "@material-ui/icons/LaunchOutlined"
import { Stack } from "components/Stack/Stack"
import React from "react"

export const Header: React.FC<{
  title: string | JSX.Element
  description: string | JSX.Element
  docsHref: string
}> = ({ title, description, docsHref }) => {
  const styles = useStyles()

  return (
    <Stack alignItems="baseline" direction="row" justifyContent="space-between">
      <div className={styles.headingGroup}>
        <h1 className={styles.title}>{title}</h1>
        <span className={styles.description}>{description}</span>
      </div>

      <Button
        size="small"
        startIcon={<LaunchOutlined />}
        component="a"
        href={docsHref}
        target="_blank"
        variant="outlined"
      >
        Read the docs
      </Button>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  headingGroup: {
    maxWidth: 420,
    marginBottom: theme.spacing(3),
  },

  title: {
    fontSize: 32,
    fontWeight: 700,
    display: "flex",
    alignItems: "center",
    lineHeight: "initial",
    margin: 0,
    marginBottom: theme.spacing(0.5),
    gap: theme.spacing(1),
  },

  description: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
  },
}))
