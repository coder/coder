import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { FormCloseButton } from "../FormCloseButton/FormCloseButton"
import { FormTitle } from "../FormTitle/FormTitle"
import { Margins } from "../Margins/Margins"

export interface FullPageFormProps {
  title: string
  detail?: React.ReactNode
  onCancel: () => void
}

const useStyles = makeStyles(() => ({
  root: {
    width: "100%",
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
  },
}))

export const FullPageForm: React.FC<FullPageFormProps> = ({ title, detail, onCancel, children }) => {
  const styles = useStyles()
  return (
    <main className={styles.root}>
      <Margins>
        <FormTitle title={title} detail={detail} />
        <FormCloseButton onClose={onCancel} />

        {children}
      </Margins>
    </main>
  )
}
