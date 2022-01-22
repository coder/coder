import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { ButtonProps } from "@material-ui/core/Button"
import React from "react"

import { Title } from "../Form"

const useStyles = makeStyles(() => ({
  form: {
    display: "flex",
    flexDirection: "column",
    flex: "1 1 auto",
  },
  header: {
    flex: "0",
    marginTop: "1em",
  },
  body: {
    padding: "2em",
    flex: "1",
    overflowY: "auto",
    display: "flex",
    flexDirection: "column",
    justifyContent: "flex-start",
    alignItems: "center",
  },
  footer: {
    display: "flex",
    flex: "0",
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
  },
  button: {
    margin: "1em",
  },
}))

export interface FormButton {
  props: ButtonProps
  title: string
}

export interface FormPageProps {
  title: string
  detail?: React.ReactNode
  buttons?: FormButton[]
}

export const FormPage: React.FC<FormPageProps> = ({ title, detail, children, buttons }) => {
  const styles = useStyles()

  const actualButtons = buttons || []

  return (
    <div className={styles.form}>
      <div className={styles.header}>
        <Title title={title} detail={detail} />
      </div>
      <div className={styles.body}>{children}</div>
      <div className={styles.footer}>
        {actualButtons.map(({ props, title }) => {
          return (
            <Button {...props} className={styles.button}>
              {title}
            </Button>
          )
        })}
      </div>
    </div>
  )
}
